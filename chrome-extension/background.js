// Background script for auto-syncing repository activity
const DEFAULT_API_BASE_URL = "https://gitsense-ooly.onrender.com";

// ----------------------------
// MESSAGE LISTENER FROM POPUP
// ----------------------------
chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type === "SET_TOKEN") {
    chrome.storage.local.set({ authToken: msg.token });
    console.log("✅ Token saved for background sync");
  }

  if (msg.type === "SET_REPO") {
    chrome.storage.local.set({ selectedRepo: msg.repo });
    console.log("✅ Repository set for background sync:", msg.repo);
  }

  if (msg.type === "CLEAR") {
    chrome.storage.local.remove(['authToken', 'selectedRepo']);
    console.log("✅ Background sync cleared");
  }
});

// ----------------------------
// AUTO-SYNC FUNCTION
// ----------------------------
async function autoSync() {
  // Get token and repo from storage
  const result = await chrome.storage.local.get(['authToken', 'selectedRepo', 'apiBaseUrl']);

  const { authToken, selectedRepo } = result;
  const apiBaseUrl = result.apiBaseUrl || DEFAULT_API_BASE_URL;

  // Exit if not authenticated or no repo selected
  if (!authToken || !selectedRepo) {
    return;
  }

  const [owner, repo] = selectedRepo.split("/");

  console.log(`🔄 Auto-syncing ${selectedRepo}...`);

  try {
    // Sync the repository
    const response = await fetch(`${apiBaseUrl}/sync?owner=${owner}&repo=${repo}`, {
      headers: {
        Authorization: `Bearer ${authToken}`
      }
    });

    const contentType = response.headers.get("Content-Type") || "";

    // Handle legacy plain-text responses (backward compatibility)
    if (!contentType.includes("application/json")) {
      const msg = await response.text();
      if (msg.includes("🔔")) {
        chrome.notifications.create({
          type: "basic",
          iconUrl: "icon.png",
          title: "GitSense – New Commit",
          message: msg
        });
      } else {
        console.log("✅ Auto-sync completed (legacy response, no activity)");
      }
      return;
    }

    // Parse structured JSON response
    const syncData = await response.json();

    if (!syncData || !syncData.notifications) {
      console.log("✅ Auto-sync completed (no notifications)");
      return;
    }

    // Load notification preferences and watched files
    const prefs = await chrome.storage.local.get(['notificationPrefs', 'watchedFiles']);
    const notifPrefs = prefs.notificationPrefs || {
      new_commits: true,
      activity_spike: true,
      repo_inactive: true,
      watched_file: true
    };

    // Fire backend-generated notifications based on user preferences
    for (const notif of syncData.notifications) {
      if (notifPrefs[notif.type]) {
        chrome.notifications.create({
          type: "basic",
          iconUrl: "icon.png",
          title: notif.title,
          message: notif.message
        });
      }
    }

    // Client-side watched file matching
    if (notifPrefs.watched_file && syncData.updated_files && prefs.watchedFiles) {
      const watchedFiles = prefs.watchedFiles || [];
      const matchedFiles = syncData.updated_files.filter(f =>
        watchedFiles.some(watched => f.includes(watched))
      );

      if (matchedFiles.length > 0) {
        chrome.notifications.create({
          type: "basic",
          iconUrl: "icon.png",
          title: "Watched File Updated",
          message: matchedFiles.slice(0, 3).join(", ") + (matchedFiles.length > 3 ? "..." : "")
        });
      }
    }

    console.log(`✅ Auto-sync completed: ${syncData.new_commits} new commits, score: ${syncData.activity.current_score}`);
  } catch (error) {
    console.error("❌ Auto-sync failed:", error);
  }
}

// ----------------------------
// RUN AUTO-SYNC EVERY 5 MINUTES
// ----------------------------
setInterval(autoSync, 300000); // 300000 ms = 5 minutes

// Run sync immediately when background script starts (for testing)
autoSync();

console.log("🚀 GitSense background sync initialized (interval: 5 minutes)");
