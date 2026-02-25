package github

import (
	"encoding/json"
	"fmt"

	"gitsense"
	"gitsense/internal/db"
)

// ----------------------------
// GitHub API MODELS
// ----------------------------
type GitHubCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type GitHubFile struct {
	Filename string `json:"filename"`
}

// ----------------------------
// SYNC FROM GITHUB
// ----------------------------
func SyncFromGitHub(owner, repo, token string) ([]string, error) {

	// Limit commits to avoid timeout
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/commits?per_page=%d",
		owner, repo, gitsense.DefaultCommitLimit,
	)

	req, err := gitsense.CreateGitHubRequest("GET", url, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create commit request: %w", err)
	}

	client := gitsense.CreateHTTPClient(gitsense.GitHubAPITimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var commits []GitHubCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, err
	}

	fmt.Printf("📊 Found %d commits\n", len(commits))

	// Track unique filenames across all processed commits
	updatedFilesMap := make(map[string]bool)

	// ----------------------------
	// PROCESS EACH COMMIT
	// ----------------------------
	for _, c := range commits {

		// 🔹 Save commit into commits table
		_, err := db.DB.Exec(`
			INSERT OR IGNORE INTO commits
			(repo_name, commit_sha, author, message, commit_date)
			VALUES (?, ?, ?, ?, ?)
		`,
			repo,
			c.SHA,
			c.Commit.Author.Name,
			c.Commit.Message,
			c.Commit.Author.Date,
		)

		if err != nil {
			fmt.Printf(" ⚠️  Commit insertion error: %v\n", err)
		}

		// Fetch files changed in this commit
		fileURL := fmt.Sprintf(
			"https://api.github.com/repos/%s/%s/commits/%s",
			owner, repo, c.SHA,
		)

		req, err := gitsense.CreateGitHubRequest("GET", fileURL, token)
		if err != nil {
			fmt.Printf(" ⚠️  Failed to create file request for %s: %v\n", c.SHA[:7], err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf(" ⚠️  Failed to fetch files for %s: %v\n", c.SHA[:7], err)
			continue
		}
		defer resp.Body.Close()

		var detail struct {
			Files []GitHubFile `json:"files"`
		}

		err = json.NewDecoder(resp.Body).Decode(&detail)
		if err != nil {
			fmt.Printf(" ⚠️  Failed to decode files for %s: %v\n", c.SHA[:7], err)
			continue
		}

		fmt.Printf(" Commit %s: %d files\n", c.SHA[:7], len(detail.Files))

		// ----------------------------
		// UPDATE FILE ACTIVITY
		// ----------------------------
		for _, f := range detail.Files {
			updatedFilesMap[f.Filename] = true

			_, err := db.DB.Exec(`
				INSERT INTO file_activity
				(repo_name, file_name, commit_count, last_modified)
				VALUES (?, ?, 1, ?)
				ON CONFLICT(repo_name, file_name)
				DO UPDATE SET
					commit_count = commit_count + 1,
					last_modified = CASE
						WHEN excluded.last_modified > last_modified
						THEN excluded.last_modified
						ELSE last_modified
					END
			`,
				repo,
				f.Filename,
				c.Commit.Author.Date,
			)

			if err != nil {
				fmt.Printf(" ❌ DB error for %s: %v\n", f.Filename, err)
			}
		}
	}

	updatedFiles := make([]string, 0, len(updatedFilesMap))
	for fname := range updatedFilesMap {
		updatedFiles = append(updatedFiles, fname)
	}
	return updatedFiles, nil
}

// ----------------------------
// FETCH GITHUB USERNAME
// ----------------------------
func GetGitHubUsername(token string) string {
	req, err := gitsense.CreateGitHubRequest("GET", "https://api.github.com/user", token)
	if err != nil {
		fmt.Println("❌ Failed to create username request:", err)
		return ""
	}

	client := gitsense.CreateHTTPClient(gitsense.DefaultTimeout)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Failed to fetch GitHub username:", err)
		return ""
	}
	defer resp.Body.Close()

	var data struct {
		Login string `json:"login"`
	}

	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		fmt.Println("❌ Failed to decode username response:", err)
		return ""
	}

	return data.Login
}
