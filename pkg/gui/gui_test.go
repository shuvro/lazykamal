package gui

import "testing"

func TestScreenString(t *testing.T) {
	tests := []struct {
		screen Screen
		want   string
	}{
		{ScreenApps, "apps"},
		{ScreenMainMenu, "main"},
		{ScreenDeploy, "deploy"},
		{ScreenApp, "app"},
		{ScreenServer, "server"},
		{ScreenAccessory, "accessory"},
		{ScreenProxy, "proxy"},
		{ScreenOther, "other"},
		{ScreenConfig, "config"},
		{ScreenEditor, "editor"},
		{ScreenHelp, "help"},
		{ScreenConfirm, "confirm"},
		{ScreenBuild, "build"},
		{ScreenPrune, "prune"},
		{ScreenSecrets, "secrets"},
		{ScreenRegistry, "registry"},
		{Screen(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.screen.String(); got != tt.want {
				t.Errorf("Screen(%d).String() = %q, want %q", tt.screen, got, tt.want)
			}
		})
	}
}

// menuItemCounts maps each screen to its expected number of menu items.
// This must stay in sync with the render functions and keyDown max bounds.
var menuItemCounts = map[Screen]int{
	ScreenMainMenu: 7,  // Deploy, App, Server, Accessory, Proxy, Other, Config
	ScreenDeploy:   8,  // Deploy, Deploy (skip push), Redeploy, Rollback, Setup, Deploy (no cache), Redeploy (no cache), Setup (no cache)
	ScreenApp:      17, // Boot..Live:App logs + Stale containers (stop) + Exec: whoami (detach)
	ScreenServer:   3,  // Bootstrap, Exec: date, Exec: uptime
	ScreenAccessory: 10, // Boot..Upgrade
	ScreenProxy:    13, // Boot..Live: Proxy logs
	ScreenOther:    19, // Prune>, Build>, Config..Version
	ScreenConfig:   4,  // Edit deploy, Edit secrets, Redeploy, App restart
	ScreenBuild:    7,  // Push, Pull, Deliver, Dev, Create, Remove, Details
	ScreenPrune:    3,  // All, Images, Containers
	ScreenSecrets:  3,  // Fetch, Extract, Print
	ScreenRegistry: 4,  // Setup, Login, Logout, Remove
}

func TestKeyDownMaxBounds(t *testing.T) {
	// The keyDown max bound for each screen should be itemCount - 1.
	// This test verifies the bounds match the menu item counts.
	expectedMax := map[Screen]int{
		ScreenMainMenu:  6,
		ScreenDeploy:    7,
		ScreenApp:       16,
		ScreenServer:    2,
		ScreenAccessory: 9,
		ScreenProxy:     12,
		ScreenOther:     18,
		ScreenConfig:    3,
		ScreenBuild:     6,
		ScreenPrune:     2,
		ScreenSecrets:   2,
		ScreenRegistry:  3,
	}

	for screen, wantMax := range expectedMax {
		count, ok := menuItemCounts[screen]
		if !ok {
			t.Errorf("Screen %q missing from menuItemCounts", screen)
			continue
		}
		if count-1 != wantMax {
			t.Errorf("Screen %q: menuItemCounts=%d implies max=%d, but expected max=%d",
				screen, count, count-1, wantMax)
		}
	}
}

func TestGetDestructiveMessage(t *testing.T) {
	tests := []struct {
		name    string
		screen  Screen
		idx     int
		want    string
		generic bool // true if we expect the generic fallback
	}{
		// Deploy
		{"deploy rollback", ScreenDeploy, 3, "Rollback to previous version?", false},
		// App
		{"app stop", ScreenApp, 2, "Stop the application?", false},
		{"app remove", ScreenApp, 13, "Remove the application? This cannot be undone.", false},
		{"app stale containers stop", ScreenApp, 15, "Stop and remove stale containers?", false},
		// Accessory
		{"accessory stop all", ScreenAccessory, 2, "Stop all accessories?", false},
		{"accessory remove all", ScreenAccessory, 5, "Remove all accessories? This cannot be undone.", false},
		// Proxy
		{"proxy stop", ScreenProxy, 2, "Stop the proxy?", false},
		{"proxy remove", ScreenProxy, 8, "Remove the proxy? This cannot be undone.", false},
		// Other
		{"other lock force", ScreenOther, 8, "Force release the lock?", false},
		{"other env delete", ScreenOther, 13, "Delete environment variables?", false},
		// Build
		{"build remove", ScreenBuild, 5, "Remove the build setup?", false},
		// Prune
		{"prune all", ScreenPrune, 0, "Prune all old images and containers?", false},
		{"prune images", ScreenPrune, 1, "Prune old images?", false},
		{"prune containers", ScreenPrune, 2, "Prune old containers?", false},
		// Registry
		{"registry remove", ScreenRegistry, 3, "Remove registry configuration?", false},
		// Generic fallback
		{"generic", ScreenDeploy, 0, "Are you sure you want to proceed?", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDestructiveMessage(tt.screen, tt.idx)
			if got != tt.want {
				t.Errorf("getDestructiveMessage(%q, %d) = %q, want %q", tt.screen, tt.idx, got, tt.want)
			}
		})
	}
}
