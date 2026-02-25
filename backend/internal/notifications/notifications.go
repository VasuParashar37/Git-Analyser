package notifications

import "fmt"

// Notification thresholds
const (
	// ActivitySpikeThreshold is the minimum score increase to trigger a spike notification.
	ActivitySpikeThreshold = 20

	// InactivityScoreThreshold is the score at or below which a repo is considered inactive (STABLE state).
	InactivityScoreThreshold = 25
)

// NotificationEvent represents a single notification to surface to the user.
type NotificationEvent struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// ActivityInfo holds current and previous activity state for comparison.
type ActivityInfo struct {
	CurrentScore  int    `json:"current_score"`
	PreviousScore int    `json:"previous_score"`
	State         string `json:"state"`
}

// SyncResult is the structured JSON response returned from the sync endpoint.
type SyncResult struct {
	Message       string              `json:"message"`
	NewCommits    int                 `json:"new_commits"`
	UpdatedFiles  []string            `json:"updated_files"`
	Activity      ActivityInfo        `json:"activity"`
	Notifications []NotificationEvent `json:"notifications"`
}

// DetectEvents examines sync results and generates notification events.
// previousScore should be -1 if no prior snapshot exists (first sync).
func DetectEvents(repoName string, newCommits int, currentScore int, previousScore int) []NotificationEvent {
	var events []NotificationEvent

	// Trigger 1: New commits detected
	if newCommits > 0 {
		events = append(events, NotificationEvent{
			Type:    "new_commits",
			Title:   fmt.Sprintf("New Commits in %s", repoName),
			Message: fmt.Sprintf("%d new commit(s) detected", newCommits),
		})
	}

	// Only compare scores if a previous snapshot existed
	if previousScore >= 0 {
		scoreDelta := currentScore - previousScore

		// Trigger 2: Activity spike — score jumped significantly
		if scoreDelta >= ActivitySpikeThreshold {
			events = append(events, NotificationEvent{
				Type:    "activity_spike",
				Title:   fmt.Sprintf("Activity Spike in %s", repoName),
				Message: fmt.Sprintf("Activity score jumped from %d to %d (+%d points)", previousScore, currentScore, scoreDelta),
			})
		}

		// Trigger 3: Repo becomes inactive — dropped to STABLE from a higher state
		if currentScore <= InactivityScoreThreshold && previousScore > InactivityScoreThreshold {
			events = append(events, NotificationEvent{
				Type:    "repo_inactive",
				Title:   fmt.Sprintf("%s Became Inactive", repoName),
				Message: fmt.Sprintf("Activity score dropped from %d to %d. Repository is now in STABLE state.", previousScore, currentScore),
			})
		}
	}

	return events
}

// GetActivityState returns the human-readable activity state for a score.
func GetActivityState(score int) string {
	if score > 50 {
		return "HIGH ACTIVITY"
	} else if score > 25 {
		return "EVOLVING"
	}
	return "STABLE"
}
