package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/lazykamal/lazykamal/pkg/kamal"
	"golang.org/x/term"
)

const (
	viewMain    = "main"
	viewStatus  = "status"
	viewLog     = "log"
	viewHeader  = "header"
	statusPoll  = 4 * time.Second
	logBufLive  = 3000
	logBufCmd   = 500
	statusLines = 12
)

// Screen represents the current command category.
type Screen int

const (
	ScreenApps Screen = iota
	ScreenMainMenu
	ScreenDeploy
	ScreenApp
	ScreenServer
	ScreenAccessory
	ScreenProxy
	ScreenOther
	ScreenConfig
	ScreenEditor
	ScreenHelp
	ScreenConfirm
)

func (s Screen) String() string {
	switch s {
	case ScreenApps:
		return "apps"
	case ScreenMainMenu:
		return "main"
	case ScreenDeploy:
		return "deploy"
	case ScreenApp:
		return "app"
	case ScreenServer:
		return "server"
	case ScreenAccessory:
		return "accessory"
	case ScreenProxy:
		return "proxy"
	case ScreenOther:
		return "other"
	case ScreenConfig:
		return "config"
	case ScreenEditor:
		return "editor"
	case ScreenHelp:
		return "help"
	case ScreenConfirm:
		return "confirm"
	default:
		return "unknown"
	}
}

// GUI holds TUI state.
type GUI struct {
	g             *gocui.Gui
	cwd           string
	version       string
	destinations  []kamal.DeployDestination
	selectedApp   int
	screen        Screen
	prevScreen    Screen
	submenuIdx    int
	logLines      []string
	logMu         sync.Mutex
	statusText    string
	statusMu      sync.Mutex
	running       bool
	runningCmd    string
	cmdStartTime  time.Time
	maxX          int
	maxY          int
	statusStopCh  chan struct{}
	statusTicker  *time.Ticker
	liveLogsStop   chan struct{}
	liveLogsActive bool
	liveLogsMu     sync.Mutex
	savedTermState *term.State
	stdinFd        int
	editor         *editorState
	spinner        *Spinner
	confirm        *confirmState
}

// New creates a new GUI. Call FindDeployConfigs after to set destinations.
// Optionally pass version string to display in header.
func New(version ...string) (*GUI, error) {
	ver := "dev"
	if len(version) > 0 && version[0] != "" {
		ver = version[0]
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	stdinFd := int(os.Stdin.Fd())
	savedState, _ := term.GetState(stdinFd)

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return nil, err
	}
	gui := &GUI{
		stdinFd:        stdinFd,
		savedTermState: savedState,
		g:            g,
		cwd:          cwd,
		version:      ver,
		selectedApp:  0,
		screen:       ScreenApps,
		submenuIdx:   0,
		logLines:     make([]string, 0, logBufLive),
		statusStopCh: make(chan struct{}),
		liveLogsStop: make(chan struct{}),
		maxX:         80,
		maxY:        24,
	}
	gui.destinations, _ = kamal.FindDeployConfigs(gui.cwd)
	if len(gui.destinations) == 0 {
		gui.destinations = []kamal.DeployDestination{}
	}

	g.SetManagerFunc(gui.layout)
	if err := gui.keybindings(g); err != nil {
		return nil, err
	}
	g.SelFgColor = gocui.ColorCyan
	gui.startStatusPolling()
	return gui, nil
}

func (gui *GUI) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if maxX < 20 {
		maxX = 20
	}
	if maxY < 10 {
		maxY = 10
	}
	gui.maxX = maxX
	gui.maxY = maxY

	// Header
	if v, err := g.SetView(viewHeader, 0, 0, maxX-1, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Lazykamal "
		v.FgColor = gocui.ColorCyan
	}
	header, _ := g.View(viewHeader)
	header.Clear()
	gui.liveLogsMu.Lock()
	live := gui.liveLogsActive
	gui.liveLogsMu.Unlock()

	// Build status indicator
	var statusIndicator string
	if gui.running {
		elapsed := time.Since(gui.cmdStartTime)
		if gui.spinner != nil {
			statusIndicator = fmt.Sprintf(" %s %s (%s)", gui.spinner.Frame(), gui.runningCmd, formatDuration(elapsed))
		} else {
			statusIndicator = fmt.Sprintf(" %s %s (%s)", yellow(iconRunning), gui.runningCmd, formatDuration(elapsed))
		}
	} else if live {
		statusIndicator = " " + green(iconPlay) + " Live logs (Esc to stop)"
	} else {
		statusIndicator = " " + green(iconCheck) + " Ready"
	}

	// Breadcrumb navigation
	breadcrumb := gui.getBreadcrumb()

	fmt.Fprintf(header, " %s %s %s | %s |%s\n", 
		cyan(iconRocket), bold("Lazykamal"), dim(gui.version), 
		breadcrumb, statusIndicator)

	// Left panel: apps / menu (about 40% width)
	leftW := maxX * 4 / 10
	if leftW < 25 {
		leftW = 25
	}
	if v, err := g.SetView(viewMain, 0, 3, leftW-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Highlight = true
		v.SelBgColor = gocui.ColorCyan
		v.SelFgColor = gocui.ColorBlack
	}

	// Right: status (top) + log (bottom)
	statusH := statusLines + 2
	if statusH > maxY-6 {
		statusH = maxY - 6
	}
	statusY := 3 + statusH
	if v, err := g.SetView(viewStatus, leftW, 3, maxX-1, statusY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Live status "
		v.Wrap = true
	}
	if v, err := g.SetView(viewLog, leftW, statusY, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Output / Live logs "
		v.Autoscroll = true
		v.Wrap = true
	}

	if gui.screen == ScreenEditor {
		return gui.renderEditorView(g)
	}
	if gui.screen == ScreenHelp {
		return gui.renderHelpOverlay(g)
	}
	if gui.screen == ScreenConfirm {
		gui.renderLeftPanel(g)
		gui.renderStatus(g)
		gui.renderLog(g)
		return gui.renderConfirmDialog(g)
	}
	gui.renderLeftPanel(g)
	gui.renderStatus(g)
	gui.renderLog(g)
	return nil
}

const viewHelp = "help"

func (gui *GUI) renderHelpOverlay(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	
	// Center the help overlay
	width := 60
	height := 22
	if width > maxX-4 {
		width = maxX - 4
	}
	if height > maxY-4 {
		height = maxY - 4
	}
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2
	x1 := x0 + width
	y1 := y0 + height

	if v, err := g.SetView(viewHelp, x0, y0, x1, y1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Lazykamal Help "
		v.Wrap = true
	}
	v, _ := g.View(viewHelp)
	if v == nil {
		return nil
	}
	v.Clear()
	
	help := `
 KEYBOARD SHORTCUTS
 ══════════════════════════════════════════════

 Navigation
 ──────────────────────────────────────────────
   ↑/↓         Move up/down in menus
   Enter       Select item / Execute command
   Esc / b     Go back to previous screen
   m           Jump to main menu
   q / Ctrl+C  Quit application

 Actions
 ──────────────────────────────────────────────
   r           Refresh destinations and status
   c           Clear output/log panel

 Live Logs
 ──────────────────────────────────────────────
   Esc         Stop live log streaming

 Editor (when editing files)
 ──────────────────────────────────────────────
   ↑/↓/←/→     Move cursor
   Ctrl+S      Save file
   Ctrl+Q/Esc  Quit editor (prompts if unsaved)

 Help
 ──────────────────────────────────────────────
   ?           Show this help overlay
   Esc / ?     Close help

 Press Esc or ? to close this help
`
	fmt.Fprint(v, help)
	g.SetCurrentView(viewHelp)
	return nil
}

func (gui *GUI) renderStatus(g *gocui.Gui) {
	v, err := g.View(viewStatus)
	if err != nil || v == nil {
		return
	}
	v.Clear()
	gui.statusMu.Lock()
	text := gui.statusText
	gui.statusMu.Unlock()
	if text == "" {
		fmt.Fprintln(v, " Polling app version & containers...")
		return
	}
	fmt.Fprint(v, text)
}

func (gui *GUI) renderLeftPanel(g *gocui.Gui) {
	v, err := g.View(viewMain)
	if err != nil || v == nil {
		return
	}
	v.Clear()
	switch gui.screen {
	case ScreenApps:
		gui.renderApps(v)
	case ScreenMainMenu:
		gui.renderMainMenu(v)
	case ScreenDeploy:
		gui.renderDeployMenu(v)
	case ScreenApp:
		gui.renderAppMenu(v)
	case ScreenServer:
		gui.renderServerMenu(v)
	case ScreenAccessory:
		gui.renderAccessoryMenu(v)
	case ScreenProxy:
		gui.renderProxyMenu(v)
	case ScreenOther:
		gui.renderOtherMenu(v)
	case ScreenConfig:
		gui.renderConfigMenu(v)
	}
}

func (gui *GUI) renderApps(v *gocui.View) {
	v.Title = " Apps (destinations) "
	if len(gui.destinations) == 0 {
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " No config/deploy*.yml found.")
		fmt.Fprintln(v, " Run from a Kamal app root.")
		return
	}
	for i, d := range gui.destinations {
		prefix := "  "
		if i == gui.selectedApp {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, d.Label())
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " ↑/↓ select  Enter: commands")
}

func (gui *GUI) renderMainMenu(v *gocui.View) {
	v.Title = " Commands "
	items := []string{
		"Deploy / Redeploy / Rollback",
		"App (boot, start, stop, logs…)",
		"Server (bootstrap, exec)",
		"Accessory (boot, logs, reboot)",
		"Proxy (boot, logs, reboot)",
		"Other (prune, config, lock…)",
		"Config (edit deploy.yml, secrets, restart)",
	}
	for i, s := range items {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, s)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: open  Esc: back")
}

func (gui *GUI) renderDeployMenu(v *gocui.View) {
	v.Title = " Deploy "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{"Deploy", "Deploy (skip push)", "Redeploy", "Rollback", "Setup (first-time)"}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: run  b/Esc: back")
}

func (gui *GUI) renderAppMenu(v *gocui.View) {
	v.Title = " App "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{"Boot", "Start", "Stop", "Restart", "Logs", "Containers", "Details", "Images", "Version", "Stale containers", "Exec: whoami", "Maintenance", "Live", "Remove", "Live: App logs (stream)"}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: run  b/Esc: back")
}

func (gui *GUI) renderServerMenu(v *gocui.View) {
	v.Title = " Server "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{"Bootstrap", "Exec: date", "Exec: uptime"}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: run  b/Esc: back")
}

func (gui *GUI) renderAccessoryMenu(v *gocui.View) {
	v.Title = " Accessory "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{"Boot all", "Start all", "Stop all", "Restart all", "Reboot all", "Remove all", "Details all", "Logs all", "Upgrade"}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: run  b/Esc: back")
}

func (gui *GUI) renderProxyMenu(v *gocui.View) {
	v.Title = " Proxy "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{"Boot", "Start", "Stop", "Restart", "Reboot", "Reboot (rolling)", "Logs", "Details", "Remove", "Boot config get", "Boot config set", "Boot config reset", "Live: Proxy logs (stream)"}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: run  b/Esc: back")
}

func (gui *GUI) renderOtherMenu(v *gocui.View) {
	v.Title = " Other "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{"Prune", "Build", "Config", "Details", "Audit", "Lock status", "Lock acquire", "Lock release", "Lock release --force", "Registry login", "Registry logout", "Secrets", "Env push", "Env pull", "Env delete", "Docs", "Help", "Init", "Upgrade", "Version"}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " Enter: run  b/Esc: back  ?: help")
}

func (gui *GUI) renderConfigMenu(v *gocui.View) {
	v.Title = " Config "
	dest := gui.selectedDestination()
	label := "—"
	if dest != nil {
		label = dest.Label()
	}
	fmt.Fprintf(v, " App: %s\n\n", label)
	actions := []string{
		"Edit deploy config (current dest)",
		"Edit secrets (current dest)",
		"Redeploy (after edit)",
		"App restart (after edit)",
	}
	for i, a := range actions {
		prefix := "  "
		if i == gui.submenuIdx {
			prefix = "› "
		}
		fmt.Fprintf(v, "%s%s\n", prefix, a)
	}
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " In-TUI edit (nano/vi style)  b/Esc: back")
}

func (gui *GUI) renderLog(g *gocui.Gui) {
	v, err := g.View(viewLog)
	if err != nil || v == nil {
		return
	}
	v.Clear()
	gui.logMu.Lock()
	lines := append([]string(nil), gui.logLines...)
	gui.logMu.Unlock()
	if len(lines) == 0 {
		fmt.Fprintln(v, " Command output will appear here.")
		return
	}
	start := 0
	if len(lines) > gui.maxY-6 {
		start = len(lines) - (gui.maxY - 6)
	}
	for _, l := range lines[start:] {
		fmt.Fprintln(v, l)
	}
}

func (gui *GUI) selectedDestination() *kamal.DeployDestination {
	if len(gui.destinations) == 0 {
		return nil
	}
	if gui.selectedApp < 0 || gui.selectedApp >= len(gui.destinations) {
		return nil
	}
	return &gui.destinations[gui.selectedApp]
}

func (gui *GUI) runOpts() kamal.RunOptions {
	return kamal.RunOpts(gui.cwd, gui.selectedDestination())
}

func (gui *GUI) startStatusPolling() {
	gui.statusTicker = time.NewTicker(statusPoll)
	go func() {
		for {
			select {
			case <-gui.statusStopCh:
				return
			case <-gui.statusTicker.C:
				gui.refreshStatus()
			}
		}
	}()
}

func (gui *GUI) refreshStatus() {
	dest := gui.selectedDestination()
	if dest == nil {
		gui.statusMu.Lock()
		gui.statusText = " No app selected.\n Select an app (destination) for live status."
		gui.statusMu.Unlock()
		gui.g.Update(func(*gocui.Gui) error { return nil })
		return
	}
	opts := gui.runOpts()
	var buf string
	buf = " App: " + dest.Label() + "\n\n"
	if r, err := kamal.AppVersion(opts); err == nil {
		buf += " Version:\n " + stringsTrim(r.Combined(), 2) + "\n\n"
	} else {
		buf += " Version: (error)\n\n"
	}
	if r, err := kamal.AppContainers(opts); err == nil {
		buf += " Containers:\n " + stringsTrim(r.Combined(), 8) + "\n"
	} else {
		buf += " Containers: (error)\n"
	}
	gui.statusMu.Lock()
	gui.statusText = buf
	gui.statusMu.Unlock()
	gui.g.Update(func(*gocui.Gui) error { return nil })
}

func stringsTrim(s string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i := range lines {
		lines[i] = " " + strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (gui *GUI) appendLog(lines []string) {
	gui.logMu.Lock()
	defer gui.logMu.Unlock()
	for _, line := range lines {
		// Add timestamp to each line
		gui.logLines = append(gui.logLines, timestampedLine(line))
	}
	if len(gui.logLines) > logBufLive {
		gui.logLines = gui.logLines[len(gui.logLines)-logBufLive:]
	}
}

// appendLogRaw appends lines without timestamp (for continued output)
func (gui *GUI) appendLogRaw(lines []string) {
	gui.logMu.Lock()
	defer gui.logMu.Unlock()
	gui.logLines = append(gui.logLines, lines...)
	if len(gui.logLines) > logBufLive {
		gui.logLines = gui.logLines[len(gui.logLines)-logBufLive:]
	}
}

// logSuccess appends a success message
func (gui *GUI) logSuccess(msg string) {
	gui.appendLog([]string{statusLine("success", msg)})
}

// logError appends an error message
func (gui *GUI) logError(msg string) {
	gui.appendLog([]string{statusLine("error", msg)})
}

// logInfo appends an info message
func (gui *GUI) logInfo(msg string) {
	gui.appendLog([]string{statusLine("info", msg)})
}

// logWarning appends a warning message
func (gui *GUI) logWarning(msg string) {
	gui.appendLog([]string{statusLine("warning", msg)})
}

func (gui *GUI) appendLogFromResult(r kamal.Result) {
	gui.appendLog(r.Lines())
}

func (gui *GUI) startLiveLogs(kind string) {
	gui.liveLogsMu.Lock()
	if gui.liveLogsActive {
		gui.liveLogsMu.Unlock()
		return
	}
	gui.liveLogsActive = true
	gui.liveLogsStop = make(chan struct{})
	stopCh := gui.liveLogsStop
	gui.liveLogsMu.Unlock()

	opts := gui.runOpts()
	var subcommand []string
	switch kind {
	case "app":
		subcommand = []string{"app", "logs"}
	case "proxy":
		subcommand = []string{"proxy", "logs"}
	case "accessory":
		subcommand = []string{"accessory", "logs", "all"}
	default:
		gui.liveLogsMu.Lock()
		gui.liveLogsActive = false
		gui.liveLogsMu.Unlock()
		return
	}

	lastUpdate := time.Now()
	throttle := 80 * time.Millisecond
	onLine := func(line string) {
		gui.appendLog([]string{line})
		if time.Since(lastUpdate) < throttle {
			return
		}
		lastUpdate = time.Now()
		gui.g.Update(func(*gocui.Gui) error { return nil })
	}
	go func() {
		_ = kamal.RunKamalStream(subcommand, opts, onLine, stopCh)
		gui.liveLogsMu.Lock()
		gui.liveLogsActive = false
		gui.liveLogsMu.Unlock()
		gui.g.Update(func(*gocui.Gui) error { return nil })
	}()
}

func (gui *GUI) stopLiveLogs() {
	gui.liveLogsMu.Lock()
	defer gui.liveLogsMu.Unlock()
	if !gui.liveLogsActive {
		return
	}
	close(gui.liveLogsStop)
	gui.liveLogsActive = false
}

// runEditor opens path in $EDITOR (or nano/vim). Restores terminal to cooked mode,
// runs the editor, then restores raw mode so the TUI continues.
// If no editor is available (e.g. running on a server without nano/vim) or not a TTY,
// it appends the file path to the log so you can edit elsewhere (e.g. locally and scp back).
func (gui *GUI) runEditor(path string) bool {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if p, err := exec.LookPath("nano"); err == nil {
			editor = p
		} else if p, err := exec.LookPath("vim"); err == nil {
			editor = p
		} else if p, err := exec.LookPath("vi"); err == nil {
			editor = p
		}
	}
	if editor == "" {
		gui.appendLog([]string{
			"No editor found (set EDITOR or install nano/vim).",
			"Edit the file elsewhere and save, then use Redeploy or App restart:",
			"  " + path,
		})
		return false
	}
	// Not a TTY (e.g. no terminal when SSH non-interactive) – can't run an interactive editor
	if gui.savedTermState == nil {
		gui.appendLog([]string{
			"Not a terminal; cannot run an interactive editor.",
			"Edit the file on your machine and re-upload, then Redeploy or App restart:",
			"  " + path,
		})
		return false
	}
	rawState, err := term.GetState(gui.stdinFd)
	if err != nil {
		gui.appendLog([]string{"Could not save terminal state: " + err.Error(), "File: " + path})
		return false
	}
	defer term.Restore(gui.stdinFd, rawState)
	if err := term.Restore(gui.stdinFd, gui.savedTermState); err != nil {
		gui.appendLog([]string{"Could not restore terminal for editor: " + err.Error(), "File: " + path})
		return false
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		gui.appendLog([]string{"Editor exited: " + err.Error()})
		return false
	}
	return true
}

func (gui *GUI) execConfig() {
	switch gui.submenuIdx {
	case 0: // Edit deploy config (in-TUI editor)
		dest := gui.selectedDestination()
		path := filepath.Join(gui.cwd, "config", "deploy.yml")
		if dest != nil {
			path = dest.ConfigPath
		}
		// Validate path is within the project directory
		if err := validatePath(gui.cwd, path); err != nil {
			gui.logError("Security: " + err.Error())
			return
		}
		if _, err := os.Stat(path); err != nil {
			gui.appendLog([]string{"Config not found: " + path})
			return
		}
		if gui.openEditor(path) {
			gui.appendLog([]string{"Editing " + path + " (^S save, ^Q/Esc quit)"})
		}
	case 1: // Edit secrets (in-TUI editor)
		path := kamal.SecretsPath(gui.cwd, gui.selectedDestination())
		dir := filepath.Dir(path)
		// Use 0700 for .kamal directory since it contains secrets
		if err := os.MkdirAll(dir, 0700); err != nil {
			gui.appendLog([]string{"Could not create .kamal: " + err.Error()})
			return
		}
		// Validate path is within the project directory
		if err := validatePath(gui.cwd, path); err != nil {
			gui.logError("Security: " + err.Error())
			return
		}
		if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
			// Create secrets file with secure permissions (0600)
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
				f.Close()
			}
		}
		if gui.openEditor(path) {
			gui.appendLog([]string{"Editing " + path + " (^S save, ^Q/Esc quit)"})
		}
	case 2: // Redeploy
		opts := gui.runOpts()
		gui.running = true
		go func() {
			defer func() { gui.running = false }()
			res, err := kamal.Redeploy(opts)
			if err != nil {
				gui.appendLog([]string{"Error: " + err.Error()})
				return
			}
			gui.appendLogFromResult(res)
			gui.g.Update(func(*gocui.Gui) error { return nil })
		}()
	case 3: // App restart
		opts := gui.runOpts()
		gui.running = true
		go func() {
			defer func() { gui.running = false }()
			res, err := kamal.AppRestart(opts)
			if err != nil {
				gui.appendLog([]string{"Error: " + err.Error()})
				return
			}
			gui.appendLogFromResult(res)
			gui.g.Update(func(*gocui.Gui) error { return nil })
		}()
	}
}

func (gui *GUI) refreshDestinations() {
	dests, err := kamal.FindDeployConfigs(gui.cwd)
	if err == nil {
		gui.destinations = dests
		if gui.selectedApp >= len(gui.destinations) {
			gui.selectedApp = len(gui.destinations) - 1
			if gui.selectedApp < 0 {
				gui.selectedApp = 0
			}
		}
	}
}

func (gui *GUI) keybindings(g *gocui.Gui) error {
	quit := func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'q', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}); err != nil {
		return err
	}

	// Global: m = main menu from anywhere (except apps)
	if err := g.SetKeybinding("", 'm', gocui.ModNone, gui.keyMain); err != nil {
		return err
	}
	// Global: ? = show help overlay
	if err := g.SetKeybinding("", '?', gocui.ModNone, gui.keyHelp); err != nil {
		return err
	}
	// Global: r = refresh destinations
	if err := g.SetKeybinding("", 'r', gocui.ModNone, gui.keyRefresh); err != nil {
		return err
	}
	// Global: c = clear log
	if err := g.SetKeybinding("", 'c', gocui.ModNone, gui.keyClearLog); err != nil {
		return err
	}
	// Confirm dialog: left/right arrows and enter
	if err := g.SetKeybinding(viewConfirm, gocui.KeyArrowLeft, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirmLeft()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirm, gocui.KeyArrowRight, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirmRight()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirm, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirmEnter()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirm, gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.closeConfirm()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirm, 'n', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.closeConfirm()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewConfirm, 'y', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirm.Selected = 0
		gui.confirmEnter()
		return nil
	}); err != nil {
		return err
	}
	// Up/Down
	if err := g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, gui.keyDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, gui.keyUp); err != nil {
		return err
	}
	// Enter
	if err := g.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, gui.keyEnter); err != nil {
		return err
	}
	// Escape / b = back
	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, gui.keyBack); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'b', gocui.ModNone, gui.keyBack); err != nil {
		return err
	}
	gui.setEditorKeybindings(g)
	return nil
}

func (gui *GUI) setEditorKeybindings(g *gocui.Gui) {
	// Editor view keybindings (nano/vi style). View "editor" is created when screen is ScreenEditor.
	ed := viewEditor
	bind := func(key gocui.Key, mod gocui.Modifier, fn func(*gocui.Gui, *gocui.View) error) {
		_ = g.SetKeybinding(ed, key, mod, fn)
	}
	bind(gocui.KeyArrowUp, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorMoveUp(); return nil })
	bind(gocui.KeyArrowDown, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorMoveDown(); return nil })
	bind(gocui.KeyArrowLeft, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorMoveLeft(); return nil })
	bind(gocui.KeyArrowRight, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorMoveRight(); return nil })
	bind(gocui.KeyEnter, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorEnter(); return nil })
	bind(gocui.KeyBackspace, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorBackspace(); return nil })
	bind(gocui.KeyBackspace2, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorBackspace(); return nil })
	bind(gocui.KeyEsc, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorQuit(); return nil })
	bind(gocui.KeyCtrlS, gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
		if gui.editorSave() {
			gui.appendLog([]string{"Saved " + gui.editor.Path})
		}
		return nil
	})
	bind(gocui.KeyCtrlQ, gocui.ModNone, func(*gocui.Gui, *gocui.View) error { gui.editorQuit(); return nil })
	// Printable runes for insert; y/n when ConfirmQuit trigger confirm
	for r := rune(32); r < 127; r++ {
		r := r
		bind(gocui.Key(r), gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
			if gui.editor != nil && gui.editor.ConfirmQuit {
				if r == 'y' {
					gui.editorConfirmQuitYes()
				} else {
					gui.editorConfirmQuitNo()
				}
			} else {
				gui.editorInsertRune(r)
			}
			return nil
		})
	}
}

func (gui *GUI) keyMain(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenApps {
		gui.screen = ScreenMainMenu
		gui.submenuIdx = 0
	}
	return nil
}

func (gui *GUI) keyHelp(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenHelp {
		gui.closeHelp(g)
		return nil
	}
	if gui.screen != ScreenEditor {
		gui.screen = ScreenHelp
	}
	return nil
}

func (gui *GUI) closeHelp(g *gocui.Gui) {
	g.DeleteView(viewHelp)
	gui.screen = ScreenApps
	g.SetCurrentView(viewMain)
}

// getBreadcrumb returns the current navigation path
func (gui *GUI) getBreadcrumb() string {
	dest := gui.selectedDestination()
	destLabel := dim("(no app)")
	if dest != nil {
		destLabel = cyan(dest.Label())
	}

	path := destLabel
	switch gui.screen {
	case ScreenApps:
		path = dim("Apps")
	case ScreenMainMenu:
		path = destLabel + dim(" > ") + "Menu"
	case ScreenDeploy:
		path = destLabel + dim(" > ") + yellow("Deploy")
	case ScreenApp:
		path = destLabel + dim(" > ") + green("App")
	case ScreenServer:
		path = destLabel + dim(" > ") + blue("Server")
	case ScreenAccessory:
		path = destLabel + dim(" > ") + cyan("Accessory")
	case ScreenProxy:
		path = destLabel + dim(" > ") + cyan("Proxy")
	case ScreenOther:
		path = destLabel + dim(" > ") + "Other"
	case ScreenConfig:
		path = destLabel + dim(" > ") + yellow("Config")
	}
	return path
}

func (gui *GUI) keyRefresh(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenEditor || gui.screen == ScreenHelp {
		return nil
	}
	gui.refreshDestinations()
	gui.refreshStatus()
	gui.appendLog([]string{"Refreshed destinations and status."})
	return nil
}

func (gui *GUI) keyClearLog(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenEditor || gui.screen == ScreenHelp {
		return nil
	}
	gui.logMu.Lock()
	gui.logLines = make([]string, 0, logBufLive)
	gui.logMu.Unlock()
	return nil
}

func (gui *GUI) keyBack(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenConfirm {
		gui.closeConfirm()
		return nil
	}
	if gui.screen == ScreenHelp {
		gui.closeHelp(g)
		return nil
	}
	if gui.screen == ScreenEditor {
		gui.editorQuit()
		return nil
	}
	gui.liveLogsMu.Lock()
	live := gui.liveLogsActive
	gui.liveLogsMu.Unlock()
	if live {
		gui.stopLiveLogs()
		return nil
	}
	switch gui.screen {
	case ScreenMainMenu:
		gui.screen = ScreenApps
		gui.submenuIdx = 0
	case ScreenDeploy, ScreenApp, ScreenServer, ScreenAccessory, ScreenProxy, ScreenOther, ScreenConfig:
		gui.screen = ScreenMainMenu
		gui.submenuIdx = 0
	case ScreenEditor:
		gui.editorQuit()
	}
	return nil
}

func (gui *GUI) keyUp(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenEditor {
		gui.editorMoveUp()
		return nil
	}
	switch gui.screen {
	case ScreenApps:
		if gui.selectedApp > 0 {
			gui.selectedApp--
		}
	case ScreenMainMenu:
		if gui.submenuIdx > 0 {
			gui.submenuIdx--
		}
	case ScreenDeploy:
		if gui.submenuIdx > 0 {
			gui.submenuIdx--
		}
	case ScreenApp:
		if gui.submenuIdx > 0 {
			gui.submenuIdx--
		}
	case ScreenServer, ScreenAccessory, ScreenProxy, ScreenOther, ScreenConfig:
		if gui.submenuIdx > 0 {
			gui.submenuIdx--
		}
	}
	return nil
}

func (gui *GUI) keyDown(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ScreenEditor {
		gui.editorMoveDown()
		return nil
	}
	switch gui.screen {
	case ScreenApps:
		if gui.selectedApp < len(gui.destinations)-1 {
			gui.selectedApp++
		}
	case ScreenMainMenu:
		if gui.submenuIdx < 6 {
			gui.submenuIdx++
		}
	case ScreenDeploy:
		if gui.submenuIdx < 4 {
			gui.submenuIdx++
		}
	case ScreenApp:
		if gui.submenuIdx < 14 {
			gui.submenuIdx++
		}
	case ScreenServer:
		if gui.submenuIdx < 2 {
			gui.submenuIdx++
		}
	case ScreenAccessory:
		if gui.submenuIdx < 8 {
			gui.submenuIdx++
		}
	case ScreenProxy:
		if gui.submenuIdx < 12 {
			gui.submenuIdx++
		}
	case ScreenOther:
		if gui.submenuIdx < 19 {
			gui.submenuIdx++
		}
	case ScreenConfig:
		if gui.submenuIdx < 3 {
			gui.submenuIdx++
		}
	}
	return nil
}

func (gui *GUI) keyEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.running {
		return nil
	}
	switch gui.screen {
	case ScreenApps:
		gui.screen = ScreenMainMenu
		gui.submenuIdx = 0
	case ScreenMainMenu:
		gui.screen = ScreenDeploy + Screen(gui.submenuIdx)
		gui.submenuIdx = 0
	case ScreenConfig:
		gui.execConfig()
	case ScreenDeploy:
		gui.execDeploy()
	case ScreenApp:
		gui.execApp()
	case ScreenServer:
		gui.execServer()
	case ScreenAccessory:
		gui.execAccessory()
	case ScreenProxy:
		gui.execProxy()
	case ScreenOther:
		gui.execOther()
	}
	return nil
}

// runCommand executes a kamal command with spinner, timing, and proper logging
func (gui *GUI) runCommand(name string, fn func() (kamal.Result, error)) {
	gui.running = true
	gui.runningCmd = name
	gui.cmdStartTime = time.Now()
	
	// Start spinner
	gui.spinner = NewSpinner(name, func() {
		gui.g.Update(func(*gocui.Gui) error { return nil })
	})
	gui.spinner.Start()
	
	gui.logInfo("Running: " + name)
	
	go func() {
		defer func() {
			gui.spinner.Stop()
			gui.spinner = nil
			gui.running = false
			gui.runningCmd = ""
			gui.g.Update(func(*gocui.Gui) error { return nil })
		}()
		
		res, err := fn()
		duration := time.Since(gui.cmdStartTime)
		
		if err != nil {
			gui.logError(fmt.Sprintf("%s failed: %s", name, err.Error()))
			return
		}
		
		// Log output
		gui.appendLogFromResult(res)
		
		// Log completion with duration
		if res.ExitCode == 0 {
			gui.logSuccess(fmt.Sprintf("%s completed in %s", name, formatDuration(duration)))
		} else {
			gui.logError(fmt.Sprintf("%s failed (exit %d) in %s", name, res.ExitCode, formatDuration(duration)))
		}
	}()
}

// runWithConfirm shows a confirmation dialog before running a destructive command
func (gui *GUI) runWithConfirm(name string, message string, fn func() (kamal.Result, error)) {
	gui.prevScreen = gui.screen
	gui.showConfirm("Confirm "+name, message, func() {
		gui.runCommand(name, fn)
	}, nil)
}

func (gui *GUI) execDeploy() {
	opts := gui.runOpts()
	var fn func() (kamal.Result, error)
	var name string
	
	switch gui.submenuIdx {
	case 0:
		name = "Deploy"
		fn = func() (kamal.Result, error) { return kamal.Deploy(opts, false) }
	case 1:
		name = "Deploy (skip push)"
		fn = func() (kamal.Result, error) { return kamal.Deploy(opts, true) }
	case 2:
		name = "Redeploy"
		fn = func() (kamal.Result, error) { return kamal.Redeploy(opts) }
	case 3:
		name = "Rollback"
		fn = func() (kamal.Result, error) { return kamal.Rollback(opts, "") }
		gui.runWithConfirm(name, getDestructiveMessage(gui.screen, gui.submenuIdx), fn)
		return
	case 4:
		name = "Setup"
		fn = func() (kamal.Result, error) { return kamal.Setup(opts) }
	default:
		return
	}
	
	gui.runCommand(name, fn)
}

func (gui *GUI) execApp() {
	opts := gui.runOpts()
	var fn func() (kamal.Result, error)
	var name string
	needsConfirm := false
	
	switch gui.submenuIdx {
	case 0:
		name = "App Boot"
		fn = func() (kamal.Result, error) { return kamal.AppBoot(opts) }
	case 1:
		name = "App Start"
		fn = func() (kamal.Result, error) { return kamal.AppStart(opts) }
	case 2:
		name = "App Stop"
		fn = func() (kamal.Result, error) { return kamal.AppStop(opts) }
		needsConfirm = true
	case 3:
		name = "App Restart"
		fn = func() (kamal.Result, error) { return kamal.AppRestart(opts) }
	case 4:
		name = "App Logs"
		fn = func() (kamal.Result, error) { return kamal.AppLogs(opts) }
	case 5:
		name = "App Containers"
		fn = func() (kamal.Result, error) { return kamal.AppContainers(opts) }
	case 6:
		name = "App Details"
		fn = func() (kamal.Result, error) { return kamal.AppDetails(opts) }
	case 7:
		name = "App Images"
		fn = func() (kamal.Result, error) { return kamal.AppImages(opts) }
	case 8:
		name = "App Version"
		fn = func() (kamal.Result, error) { return kamal.AppVersion(opts) }
	case 9:
		name = "App Stale Containers"
		fn = func() (kamal.Result, error) { return kamal.AppStaleContainers(opts) }
	case 10:
		name = "App Exec: whoami"
		fn = func() (kamal.Result, error) { return kamal.AppExec(opts, "whoami") }
	case 11:
		name = "App Maintenance"
		fn = func() (kamal.Result, error) { return kamal.AppMaintenance(opts) }
	case 12:
		name = "App Live"
		fn = func() (kamal.Result, error) { return kamal.AppLive(opts) }
	case 13:
		name = "App Remove"
		fn = func() (kamal.Result, error) { return kamal.AppRemove(opts) }
		needsConfirm = true
	case 14:
		gui.startLiveLogs("app")
		return
	default:
		return
	}
	
	if needsConfirm {
		gui.runWithConfirm(name, getDestructiveMessage(gui.screen, gui.submenuIdx), fn)
	} else {
		gui.runCommand(name, fn)
	}
}

func (gui *GUI) execServer() {
	opts := gui.runOpts()
	var fn func() (kamal.Result, error)
	var name string
	
	switch gui.submenuIdx {
	case 0:
		name = "Server Bootstrap"
		fn = func() (kamal.Result, error) { return kamal.ServerBootstrap(opts) }
	case 1:
		name = "Server Exec: date"
		fn = func() (kamal.Result, error) { return kamal.ServerExec(opts, "date") }
	case 2:
		name = "Server Exec: uptime"
		fn = func() (kamal.Result, error) { return kamal.ServerExec(opts, "uptime") }
	default:
		return
	}
	
	gui.runCommand(name, fn)
}

func (gui *GUI) execAccessory() {
	opts := gui.runOpts()
	var fn func() (kamal.Result, error)
	var name string
	needsConfirm := false
	
	switch gui.submenuIdx {
	case 0:
		name = "Accessory Boot All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryBoot(opts, "all") }
	case 1:
		name = "Accessory Start All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryStart(opts, "all") }
	case 2:
		name = "Accessory Stop All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryStop(opts, "all") }
		needsConfirm = true
	case 3:
		name = "Accessory Restart All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryRestart(opts, "all") }
	case 4:
		name = "Accessory Reboot All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryReboot(opts, "all") }
	case 5:
		name = "Accessory Remove All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryRemove(opts, "all") }
		needsConfirm = true
	case 6:
		name = "Accessory Details All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryDetails(opts, "all") }
	case 7:
		name = "Accessory Logs All"
		fn = func() (kamal.Result, error) { return kamal.AccessoryLogs(opts, "all") }
	case 8:
		name = "Accessory Upgrade"
		fn = func() (kamal.Result, error) { return kamal.AccessoryUpgrade(opts) }
	default:
		return
	}
	
	if needsConfirm {
		gui.runWithConfirm(name, getDestructiveMessage(gui.screen, gui.submenuIdx), fn)
	} else {
		gui.runCommand(name, fn)
	}
}

func (gui *GUI) execProxy() {
	opts := gui.runOpts()
	var fn func() (kamal.Result, error)
	var name string
	needsConfirm := false
	
	switch gui.submenuIdx {
	case 0:
		name = "Proxy Boot"
		fn = func() (kamal.Result, error) { return kamal.ProxyBoot(opts) }
	case 1:
		name = "Proxy Start"
		fn = func() (kamal.Result, error) { return kamal.ProxyStart(opts) }
	case 2:
		name = "Proxy Stop"
		fn = func() (kamal.Result, error) { return kamal.ProxyStop(opts) }
		needsConfirm = true
	case 3:
		name = "Proxy Restart"
		fn = func() (kamal.Result, error) { return kamal.ProxyRestart(opts) }
	case 4:
		name = "Proxy Reboot"
		fn = func() (kamal.Result, error) { return kamal.ProxyReboot(opts, false) }
	case 5:
		name = "Proxy Reboot (rolling)"
		fn = func() (kamal.Result, error) { return kamal.ProxyReboot(opts, true) }
	case 6:
		name = "Proxy Logs"
		fn = func() (kamal.Result, error) { return kamal.ProxyLogs(opts) }
	case 7:
		name = "Proxy Details"
		fn = func() (kamal.Result, error) { return kamal.ProxyDetails(opts) }
	case 8:
		name = "Proxy Remove"
		fn = func() (kamal.Result, error) { return kamal.ProxyRemove(opts) }
		needsConfirm = true
	case 9:
		name = "Proxy Boot Config Get"
		fn = func() (kamal.Result, error) { return kamal.ProxyBootConfigGet(opts) }
	case 10:
		name = "Proxy Boot Config Set"
		fn = func() (kamal.Result, error) { return kamal.ProxyBootConfigSet(opts) }
	case 11:
		name = "Proxy Boot Config Reset"
		fn = func() (kamal.Result, error) { return kamal.ProxyBootConfigReset(opts) }
	case 12:
		gui.startLiveLogs("proxy")
		return
	default:
		return
	}
	
	if needsConfirm {
		gui.runWithConfirm(name, getDestructiveMessage(gui.screen, gui.submenuIdx), fn)
	} else {
		gui.runCommand(name, fn)
	}
}

func (gui *GUI) execOther() {
	opts := gui.runOpts()
	var fn func() (kamal.Result, error)
	var name string
	needsConfirm := false
	
	switch gui.submenuIdx {
	case 0:
		name = "Prune"
		fn = func() (kamal.Result, error) { return kamal.Prune(opts) }
		needsConfirm = true
	case 1:
		name = "Build"
		fn = func() (kamal.Result, error) { return kamal.Build(opts) }
	case 2:
		name = "Config"
		fn = func() (kamal.Result, error) { return kamal.Config(opts) }
	case 3:
		name = "Details"
		fn = func() (kamal.Result, error) { return kamal.Details(opts) }
	case 4:
		name = "Audit"
		fn = func() (kamal.Result, error) { return kamal.Audit(opts) }
	case 5:
		name = "Lock Status"
		fn = func() (kamal.Result, error) { return kamal.LockStatus(opts) }
	case 6:
		name = "Lock Acquire"
		fn = func() (kamal.Result, error) { return kamal.LockAcquire(opts) }
	case 7:
		name = "Lock Release"
		fn = func() (kamal.Result, error) { return kamal.LockRelease(opts) }
	case 8:
		name = "Lock Release (force)"
		fn = func() (kamal.Result, error) { return kamal.LockReleaseForce(opts) }
		needsConfirm = true
	case 9:
		name = "Registry Login"
		fn = func() (kamal.Result, error) { return kamal.RegistryLogin(opts) }
	case 10:
		name = "Registry Logout"
		fn = func() (kamal.Result, error) { return kamal.RegistryLogout(opts) }
	case 11:
		name = "Secrets"
		fn = func() (kamal.Result, error) { return kamal.Secrets(opts) }
	case 12:
		name = "Env Push"
		fn = func() (kamal.Result, error) { return kamal.EnvPush(opts) }
	case 13:
		name = "Env Pull"
		fn = func() (kamal.Result, error) { return kamal.EnvPull(opts) }
	case 14:
		name = "Env Delete"
		fn = func() (kamal.Result, error) { return kamal.EnvDelete(opts) }
		needsConfirm = true
	case 15:
		name = "Docs"
		fn = func() (kamal.Result, error) { return kamal.Docs(opts, "") }
	case 16:
		name = "Help"
		fn = func() (kamal.Result, error) { return kamal.Help(opts, "") }
	case 17:
		name = "Init"
		fn = func() (kamal.Result, error) { return kamal.Init(opts) }
	case 18:
		name = "Upgrade"
		fn = func() (kamal.Result, error) { return kamal.Upgrade(opts) }
	case 19:
		name = "Version"
		fn = func() (kamal.Result, error) { return kamal.Version(opts) }
	default:
		return
	}
	
	if needsConfirm {
		gui.runWithConfirm(name, getDestructiveMessage(gui.screen, gui.submenuIdx), fn)
	} else {
		gui.runCommand(name, fn)
	}
}

// Run starts the TUI main loop.
func (gui *GUI) Run() error {
	defer gui.g.Close()
	defer func() {
		close(gui.statusStopCh)
		if gui.statusTicker != nil {
			gui.statusTicker.Stop()
		}
	}()
	return gui.g.MainLoop()
}

// SetCwd sets working directory and re-scans deploy configs.
// SetCwd sets working directory and re-scans deploy configs.
// Returns an error if the path is invalid or unsafe.
func (gui *GUI) SetCwd(cwd string) error {
	absPath, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Validate the path is safe
	if err := validateCwd(absPath); err != nil {
		return err
	}

	gui.cwd = absPath
	gui.destinations, _ = kamal.FindDeployConfigs(gui.cwd)
	gui.selectedApp = 0
	return nil
}
