package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitsense/internal/db"
	"gitsense/internal/models"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("GitSense backend running 🚀"))
}

//Dummy Commit for testing

func GetProjectSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`SELECT last_modified FROM file_activity`)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	total, active, stable, inactive := 0, 0, 0, 0

	for rows.Next() {
		var lastModified string
		rows.Scan(&lastModified)

		t, err := time.Parse(time.RFC3339, lastModified)
		if err != nil {
			continue
		}

		days := time.Since(t).Hours() / 24
		total++

		if days <= 7 {
			active++
		} else if days <= 30 {
			stable++
		} else {
			inactive++
		}
	}

	score := 0.0
	if total > 0 {
		rawScore := (float64(active) / float64(total)) * 100
		score = float64(int(rawScore + 0.5)) // Round to nearest integer
	}

	state := "STABLE"
	if score > 50 {
		state = "HIGH ACTIVITY"
	} else if score > 25 {
		state = "EVOLVING"
	}

	json.NewEncoder(w).Encode(models.ProjectSummary{
		TotalFiles:    total,
		ActiveFiles:   active,
		StableFiles:   stable,
		InactiveFiles: inactive,
		ActivityScore: score,
		ProjectState:  state,
	})
}

func GetRepoHistory(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")

	if repo == "" {
		http.Error(w, "Repo required", 400)
		return
	}

	rows, err := db.DB.Query(`
		SELECT active_files, stable_files, inactive_files, activity_score, created_at
		FROM repo_snapshots
		WHERE repo_name = ?
		ORDER BY created_at ASC
	`, repo)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var history []map[string]interface{}

	for rows.Next() {
		var a, s, i int
		var score float64
		var time string

		rows.Scan(&a, &s, &i, &score, &time)

		history = append(history, map[string]interface{}{
			"active":   a,
			"stable":   s,
			"inactive": i,
			"score":    int(score), // Return as integer
			"time":     time,
		})
	}

	json.NewEncoder(w).Encode(history)
}

func GetFileActivity(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")

	if repo == "" {
		http.Error(w, "Repo required", 400)
		return
	}

	rows, err := db.DB.Query(`
		SELECT file_name, commit_count, last_modified
		FROM file_activity
		WHERE repo_name = ?
		ORDER BY last_modified DESC
	`, repo)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var files []map[string]interface{}

	for rows.Next() {
		var fileName string
		var commitCount int
		var lastModified string

		rows.Scan(&fileName, &commitCount, &lastModified)

		// Calculate file status based on last modified date
		t, _ := time.Parse(time.RFC3339, lastModified)
		days := time.Since(t).Hours() / 24

		status := "inactive"
		if days <= 7 {
			status = "active"
		} else if days <= 30 {
			status = "stable"
		}

		files = append(files, map[string]interface{}{
			"name":          fileName,
			"commits":       commitCount,
			"last_modified": lastModified,
			"status":        status,
		})
	}

	json.NewEncoder(w).Encode(files)
}

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/dashboard.html")
}

// ----------------------------
// COMMITS PER DAY AGGREGATION
// ----------------------------
func GetCommitsPerDay(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")

	if repo == "" {
		http.Error(w, "Repo required", 400)
		return
	}

	rows, err := db.DB.Query(`
		SELECT DATE(commit_date) as day, COUNT(*) as count
		FROM commits
		WHERE repo_name = ?
		GROUP BY DATE(commit_date)
		ORDER BY day DESC
		LIMIT 30
	`, repo)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var data []map[string]interface{}

	for rows.Next() {
		var day string
		var count int

		rows.Scan(&day, &count)

		data = append(data, map[string]interface{}{
			"date":  day,
			"count": count,
		})
	}

	json.NewEncoder(w).Encode(data)
}

// ----------------------------
// FILE BREAKDOWN (most modified, inactive, frequently updated)
// ----------------------------
func GetFileBreakdown(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")

	if repo == "" {
		http.Error(w, "Repo required", 400)
		return
	}

	rows, err := db.DB.Query(`
		SELECT file_name, commit_count, last_modified
		FROM file_activity
		WHERE repo_name = ?
		ORDER BY commit_count DESC
	`, repo)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	type FileBreakdown struct {
		Name         string `json:"name"`
		CommitCount  int    `json:"commit_count"`
		LastModified string `json:"last_modified"`
		Status       string `json:"status"`
	}

	var mostModified []FileBreakdown
	var inactive []FileBreakdown
	var frequentlyUpdated []FileBreakdown
	var allFiles []FileBreakdown

	for rows.Next() {
		var f FileBreakdown
		rows.Scan(&f.Name, &f.CommitCount, &f.LastModified)

		// Calculate status based on last modified date
		t, _ := time.Parse(time.RFC3339, f.LastModified)
		days := time.Since(t).Hours() / 24

		if days <= 7 {
			f.Status = "active"
		} else if days <= 30 {
			f.Status = "stable"
		} else {
			f.Status = "inactive"
		}

		allFiles = append(allFiles, f)
	}

	// Get top 10 most modified files
	for i := 0; i < len(allFiles) && i < 10; i++ {
		mostModified = append(mostModified, allFiles[i])
	}

	// Get inactive files (>30 days)
	for _, f := range allFiles {
		if f.Status == "inactive" {
			inactive = append(inactive, f)
		}
	}

	// Get frequently updated files (active with high commit count)
	for _, f := range allFiles {
		if f.Status == "active" && f.CommitCount >= 3 {
			frequentlyUpdated = append(frequentlyUpdated, f)
		}
	}

	response := map[string]interface{}{
		"most_modified":      mostModified,
		"inactive":           inactive,
		"frequently_updated": frequentlyUpdated,
	}

	json.NewEncoder(w).Encode(response)
}

// ----------------------------
// CONTRIBUTOR DISTRIBUTION
// ----------------------------
func GetContributorDistribution(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")

	if repo == "" {
		http.Error(w, "Repo required", 400)
		return
	}

	rows, err := db.DB.Query(`
		SELECT author, COUNT(*) as commit_count
		FROM commits
		WHERE repo_name = ?
		GROUP BY author
		ORDER BY commit_count DESC
	`, repo)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var contributors []map[string]interface{}

	for rows.Next() {
		var author string
		var count int

		rows.Scan(&author, &count)

		contributors = append(contributors, map[string]interface{}{
			"author":  author,
			"commits": count,
		})
	}

	json.NewEncoder(w).Encode(contributors)
}
