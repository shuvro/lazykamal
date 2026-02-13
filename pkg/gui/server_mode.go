package gui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/shuvro/lazykamal/pkg/docker"
	"github.com/shuvro/lazykamal/pkg/ssh"
)

// ContainerInfo represents a container with its role/type for display
type ContainerInfo struct {
	Container docker.Container
	Role      string // "web", "postgres", "redis", etc.
}

// ServerGUI holds server mode TUI state
type ServerGUI struct {
	g                 *gocui.Gui
	version           string
	host              string
	client            *ssh.Client
	apps              []docker.App
	selectedApp       int
	selectedItem      int             // For submenu navigation
	selectedContainer int             // For container selection
	allContainers     []ContainerInfo // Flattened list of all containers for current app
	screen            ServerScreen
	logLines          []string
	logMu             sync.Mutex
	logScroll         int
	running           bool
	runningCmd        string
	cmdStartTime      time.Time
	spinner           *Spinner
	cmdMu             sync.Mutex
	cmdStopCh         chan struct{}
	// Confirmation dialog
	confirm    *confirmState
	prevScreen ServerScreen
	// Live log streaming
	streamMu           sync.Mutex
	streamingLogs      bool
	liveLogsStop       chan struct{}
	streamingContainer string
}

// ServerScreen represents the current screen in server mode
type ServerScreen int

const (
	ServerScreenApps ServerScreen = iota
	ServerScreenAppMenu
	ServerScreenContainerSelect
	ServerScreenActionsMenu // Submenu: Start/Stop/Restart/etc
	ServerScreenProxyMenu   // Submenu: Proxy operations
	ServerScreenHelp
	ServerScreenConfirm
)

// NewServerMode creates a new server mode GUI
func NewServerMode(version, host string) (*ServerGUI, error) {
	client := ssh.NewClient(host)

	// Test connection
	fmt.Printf("Testing SSH connection to %s...\n", client.HostDisplay())
	if err := client.TestConnection(); err != nil {
		return nil, fmt.Errorf("SSH connection failed: %w\nMake sure you can run: ssh %s", err, client.HostDisplay())
	}
	fmt.Println("Connected!")

	// Discover apps
	fmt.Println("Discovering Kamal apps...")
	apps, err := docker.DiscoverApps(client)
	if err != nil {
		return nil, fmt.Errorf("failed to discover apps: %w", err)
	}
	fmt.Printf("Found %d app(s)\n", len(apps))

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return nil, err
	}

	gui := &ServerGUI{
		g:        g,
		version:  version,
		host:     host,
		client:   client,
		apps:     apps,
		screen:   ServerScreenApps,
		logLines: make([]string, 0, 1000),
	}

	// Initialize spinner with update function
	gui.spinner = NewSpinner("", func() {
		g.Update(func(g *gocui.Gui) error { return nil })
	})
	gui.spinner.Start()

	g.SetManagerFunc(gui.layout)
	g.Cursor = false
	g.Mouse = false

	if err := gui.keybindings(g); err != nil {
		return nil, err
	}

	return gui, nil
}

// Run starts the server mode GUI
func (gui *ServerGUI) Run() error {
	defer gui.g.Close()
	return gui.g.MainLoop()
}

// Close tears down the gocui instance, restoring terminal state.
func (gui *ServerGUI) Close() {
	gui.g.Close()
}

// layout manages the server mode layout
func (gui *ServerGUI) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	// Header
	if v, err := g.SetView(viewHeader, 0, 0, maxX-1, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Lazykamal "
	}
	gui.renderHeader(g)

	// Left panel (apps list or menu)
	leftW := maxX / 3
	if leftW < 30 {
		leftW = 30
	}
	if v, err := g.SetView(viewMain, 0, 3, leftW-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		// We use our own visual selection (> arrow) instead of gocui's cursor highlight
	}
	gui.renderLeftPanel(g)

	// Right panel - Status
	statusH := (maxY - 3) / 2
	if v, err := g.SetView(viewStatus, leftW, 3, maxX-1, 3+statusH); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " App Details "
		v.Wrap = true
	}
	gui.renderStatus(g)

	// Right panel - Logs
	if v, err := g.SetView(viewLog, leftW, 4+statusH, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Output / Logs "
		v.Wrap = true
		v.Autoscroll = false
	}
	gui.renderLog(g)

	// Confirm overlay
	if gui.screen == ServerScreenConfirm {
		return gui.renderConfirmDialog(g)
	}

	// Help overlay
	if gui.screen == ServerScreenHelp {
		return gui.renderHelpOverlay(g)
	}

	g.SetCurrentView(viewMain)
	return nil
}

func (gui *ServerGUI) renderHeader(g *gocui.Gui) {
	v, _ := g.View(viewHeader)
	if v == nil {
		return
	}
	v.Clear()

	gui.streamMu.Lock()
	isStreaming := gui.streamingLogs
	gui.streamMu.Unlock()

	gui.cmdMu.Lock()
	isRunning := gui.running
	cmdName := gui.runningCmd
	cmdStart := gui.cmdStartTime
	gui.cmdMu.Unlock()

	status := green("✓ Connected")
	if isStreaming {
		status = cyan(gui.spinner.Frame()) + " Streaming logs " + dim("(Esc to stop)")
	} else if isRunning {
		elapsed := time.Since(cmdStart)
		status = yellow(gui.spinner.Frame()) + " " + cmdName + " " + dim(formatDuration(elapsed)) + " " + dim("Ctrl+X cancel")
	}

	// Show mode indicator prominently
	modeLabel := yellow("[SERVER MODE]") + " " + cyan(gui.client.HostDisplay())

	fmt.Fprintf(v, " %s%s %s | %s | %s | %s",
		iconRocket, bold("Lazykamal"), dim(gui.version),
		modeLabel,
		status,
		dim("?: help"))
}

func (gui *ServerGUI) renderLeftPanel(g *gocui.Gui) {
	v, _ := g.View(viewMain)
	if v == nil {
		return
	}
	v.Clear()

	switch gui.screen {
	case ServerScreenApps:
		gui.renderAppsList(v)
	case ServerScreenAppMenu:
		gui.renderAppMenu(v)
	case ServerScreenContainerSelect:
		gui.renderContainerSelect(v)
	case ServerScreenActionsMenu:
		gui.renderActionsMenu(v)
	case ServerScreenProxyMenu:
		gui.renderProxyMenu(v)
	}
}

func (gui *ServerGUI) renderAppsList(v *gocui.View) {
	v.Title = fmt.Sprintf(" Apps on %s ", gui.client.Host)

	if len(gui.apps) == 0 {
		fmt.Fprintln(v, " No Kamal apps found on this server.")
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " Make sure apps are deployed with Kamal")
		fmt.Fprintln(v, " and Docker is running.")
		return
	}

	for i, app := range gui.apps {
		prefix := "  "
		if i == gui.selectedApp {
			prefix = cyan(iconArrow) + " "
		}

		running := docker.CountRunning(app.Containers)
		total := len(app.Containers)
		version := docker.GetAppVersion(app.Containers)

		status := green("●")
		if running == 0 {
			status = red("●")
		} else if running < total {
			status = yellow("●")
		}

		line := fmt.Sprintf("%s%s %s (%s)", prefix, status, app.Service, app.Destination)
		if version != "" && version != "unknown" {
			line += dim(fmt.Sprintf(" [%s]", truncate(version, 12)))
		}
		fmt.Fprintln(v, line)

		// Show container count and accessories
		if i == gui.selectedApp {
			fmt.Fprintf(v, "    %s Web: %d/%d containers\n", dim("├─"), running, total)
			for j, acc := range app.Accessories {
				accRunning := docker.CountRunning(acc.Containers)
				prefix := "├─"
				if j == len(app.Accessories)-1 {
					prefix = "└─"
				}
				fmt.Fprintf(v, "    %s %s: %d container(s)\n", dim(prefix), acc.Name, accRunning)
			}
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" ↑/↓ select  Enter: menu  r: refresh"))
}

func (gui *ServerGUI) renderAppMenu(v *gocui.View) {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]
	v.Title = fmt.Sprintf(" %s (%s) ", app.Service, app.Destination)

	// Clean, simple top-level menu with submenus
	// 0: Containers, 1: Logs, 2: Details, 3: Actions submenu, 4: Proxy submenu, 5: Exec, 6: Back
	menuItems := []struct {
		label   string
		submenu bool
		danger  bool
	}{
		{"Containers", true, false},    // 0 - Select individual containers
		{"Logs (live)", false, false},  // 1 - Live streaming logs
		{"Details", false, false},      // 2 - Show container details
		{"Actions", true, false},       // 3 - Submenu: start/stop/restart
		{"Proxy", true, false},         // 4 - Submenu: proxy operations
		{"Exec (shell)", false, false}, // 5 - Show SSH command
		{"Back", false, false},         // 6 - Go back
	}

	for i, item := range menuItems {
		prefix := "  "
		if i == gui.selectedItem {
			prefix = cyan(iconArrow) + " "
		}

		label := item.label
		if item.submenu {
			label += " →"
		}

		fmt.Fprintln(v, prefix+label)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" ↑/↓: navigate  Enter: select  b: back"))
}

func (gui *ServerGUI) renderActionsMenu(v *gocui.View) {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]
	v.Title = fmt.Sprintf(" %s › Actions ", app.Service)

	// Actions submenu: 0-7 items
	menuItems := []struct {
		label  string
		danger bool
	}{
		{"Boot / Reboot", false}, // 0
		{"Start", false},         // 1
		{"Stop", true},           // 2 - destructive
		{"Restart", false},       // 3
		{"Remove stopped", true}, // 4 - destructive
		{"Images", false},        // 5
		{"Version", false},       // 6
		{"Health", false},        // 7
		{"Back", false},          // 8
	}

	for i, item := range menuItems {
		prefix := "  "
		if i == gui.selectedItem {
			prefix = cyan(iconArrow) + " "
		}

		label := item.label
		if item.danger {
			label = red(label)
		}

		fmt.Fprintln(v, prefix+label)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" ↑/↓: navigate  Enter: select  b: back"))
}

func (gui *ServerGUI) renderProxyMenu(v *gocui.View) {
	v.Title = " Proxy "

	// Proxy submenu: 0-6 items
	menuItems := []struct {
		label  string
		danger bool
	}{
		{"Logs (live)", false}, // 0
		{"Details", false},     // 1
		{"Restart", false},     // 2
		{"Reboot", false},      // 3
		{"Stop", true},         // 4 - destructive
		{"Start", false},       // 5
		{"Back", false},        // 6
	}

	for i, item := range menuItems {
		prefix := "  "
		if i == gui.selectedItem {
			prefix = cyan(iconArrow) + " "
		}

		label := item.label
		if item.danger {
			label = red(label)
		}

		fmt.Fprintln(v, prefix+label)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" ↑/↓: navigate  Enter: select  b: back"))
}

func (gui *ServerGUI) renderContainerSelect(v *gocui.View) {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]
	v.Title = fmt.Sprintf(" %s - Select Container ", app.Service)

	// Build container list if not already done
	if len(gui.allContainers) == 0 {
		gui.buildContainerList()
	}

	for i, ci := range gui.allContainers {
		prefix := "  "
		if i == gui.selectedContainer {
			prefix = cyan(iconArrow) + " "
		}

		status := green("●")
		if ci.Container.State != "running" {
			status = red("●")
		}

		name := ci.Container.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}

		line := fmt.Sprintf("%s%s %s", prefix, status, name)
		if ci.Role != "" {
			line += dim(fmt.Sprintf(" [%s]", ci.Role))
		}
		fmt.Fprintln(v, line)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim("───────────────"))

	// Show actions for selected container
	if gui.selectedContainer < len(gui.allContainers) {
		ci := gui.allContainers[gui.selectedContainer]
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, dim(" Actions:"))
		fmt.Fprintln(v, "   l - View Logs")
		fmt.Fprintln(v, "   r - Restart")
		fmt.Fprintln(v, "   s - Stop")
		fmt.Fprintln(v, "   S - Start")
		if ci.Container.State != "running" {
			fmt.Fprintln(v, "   "+red("x - Remove (stopped)"))
		}
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" ↑/↓ select  b/Esc: back"))
}

func (gui *ServerGUI) buildContainerList() {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]
	gui.allContainers = nil

	// Add main app containers
	for _, c := range app.Containers {
		gui.allContainers = append(gui.allContainers, ContainerInfo{
			Container: c,
			Role:      "web",
		})
	}

	// Add accessory containers
	for _, acc := range app.Accessories {
		for _, c := range acc.Containers {
			gui.allContainers = append(gui.allContainers, ContainerInfo{
				Container: c,
				Role:      acc.Name,
			})
		}
	}
}

func (gui *ServerGUI) renderStatus(g *gocui.Gui) {
	v, _ := g.View(viewStatus)
	if v == nil {
		return
	}
	v.Clear()

	if gui.selectedApp >= len(gui.apps) {
		fmt.Fprintln(v, " Select an app to view details")
		return
	}

	app := gui.apps[gui.selectedApp]

	fmt.Fprintf(v, " Service: %s\n", bold(app.Service))
	fmt.Fprintf(v, " Destination: %s\n", app.Destination)
	fmt.Fprintf(v, " Version: %s\n", docker.GetAppVersion(app.Containers))
	fmt.Fprintf(v, " Proxy: %s\n", formatProxyStatus(app.ProxyStatus))
	fmt.Fprintln(v, "")

	// Containers
	running := docker.CountRunning(app.Containers)
	total := len(app.Containers)
	fmt.Fprintf(v, " Containers: %d/%d running\n", running, total)

	for _, c := range app.Containers {
		status := green("●")
		if c.State != "running" {
			status = red("●")
		}
		fmt.Fprintf(v, "   %s %s (%s)\n", status, truncate(c.Name, 30), c.State)
	}

	// Accessories
	if len(app.Accessories) > 0 {
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " Accessories:")
		for _, acc := range app.Accessories {
			accRunning := docker.CountRunning(acc.Containers)
			status := green("●")
			if accRunning == 0 {
				status = red("●")
			}
			fmt.Fprintf(v, "   %s %s (%d container(s))\n", status, acc.Name, len(acc.Containers))
		}
	}
}

func formatProxyStatus(status string) string {
	switch status {
	case "running":
		return green("✓ running")
	case "not running":
		return red("✗ not running")
	default:
		return yellow(status)
	}
}

func (gui *ServerGUI) renderLog(g *gocui.Gui) {
	v, _ := g.View(viewLog)
	if v == nil {
		return
	}
	v.Clear()

	// Update title based on streaming status
	gui.streamMu.Lock()
	isStreaming := gui.streamingLogs
	streamContainer := gui.streamingContainer
	gui.streamMu.Unlock()
	if isStreaming {
		v.Title = fmt.Sprintf(" LIVE: %s (Esc to stop) ", truncate(streamContainer, 20))
	} else {
		v.Title = " Output / Logs "
	}

	gui.logMu.Lock()
	lines := append([]string(nil), gui.logLines...)
	gui.logMu.Unlock()

	if len(lines) == 0 {
		fmt.Fprintln(v, " Output will appear here.")
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, dim(" Select an app and run a command"))
		return
	}

	_, viewHeight := v.Size()
	if viewHeight < 1 {
		viewHeight = 1
	}

	maxScroll := len(lines) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if gui.logScroll > maxScroll {
		gui.logScroll = maxScroll
	}
	if gui.logScroll < 0 {
		gui.logScroll = 0
	}

	start := gui.logScroll
	end := start + viewHeight
	if end > len(lines) {
		end = len(lines)
	}

	for _, l := range lines[start:end] {
		fmt.Fprintln(v, l)
	}
}

func (gui *ServerGUI) renderHelpOverlay(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	width := 60
	height := 28
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	if v, err := g.SetView(viewHelp, x0, y0, x0+width, y0+height); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Help "
	}

	v, _ := g.View(viewHelp)
	if v == nil {
		return nil
	}
	v.Clear()

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, yellow("  ╔══════════════════════════════════════════════════╗"))
	fmt.Fprintln(v, yellow("  ║")+bold("          YOU ARE IN SERVER MODE                ")+yellow("║"))
	fmt.Fprintln(v, yellow("  ╚══════════════════════════════════════════════════╝"))
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "  Server Mode connects via SSH to manage containers")
	fmt.Fprintln(v, "  using Docker commands directly on the server.")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim("  Available: logs, start, stop, restart, health, etc."))
	fmt.Fprintln(v, red("  NOT available: deploy, redeploy, rollback, build"))
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, cyan("  For deploy commands, use Project Mode:"))
	fmt.Fprintln(v, "    $ lazykamal /path/to/kamal/project")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " ──────────────────────────────────────────────────────")
	fmt.Fprintln(v, "  KEYBOARD SHORTCUTS")
	fmt.Fprintln(v, " ──────────────────────────────────────────────────────")
	fmt.Fprintln(v, "   ↑/↓       Navigate       j/k       Scroll logs")
	fmt.Fprintln(v, "   Enter     Select         c         Clear log")
	fmt.Fprintln(v, "   b/Esc     Go back        r         Refresh apps")
	fmt.Fprintln(v, "   Ctrl+X    Cancel cmd     ?         Help")
	fmt.Fprintln(v, "   q         Quit")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim("  Press ? or Esc to close"))

	return nil
}

func (gui *ServerGUI) appendLog(lines []string) {
	gui.logMu.Lock()
	defer gui.logMu.Unlock()
	for _, line := range lines {
		gui.logLines = append(gui.logLines, timestampedLine(sanitizeLogLine(line)))
	}
	if len(gui.logLines) > 1000 {
		gui.logLines = gui.logLines[len(gui.logLines)-1000:]
	}
	// Auto-scroll to bottom
	gui.logScroll = len(gui.logLines)
}

func (gui *ServerGUI) logSuccess(msg string) {
	gui.appendLog([]string{statusLine("success", msg)})
}

func (gui *ServerGUI) logError(msg string) {
	gui.appendLog([]string{statusLine("error", msg)})
}

func (gui *ServerGUI) logInfo(msg string) {
	gui.appendLog([]string{statusLine("info", msg)})
}

// cancelCommand cancels the currently running server command if any.
func (gui *ServerGUI) cancelCommand() {
	gui.cmdMu.Lock()
	var name string
	if gui.running && gui.cmdStopCh != nil {
		name = gui.runningCmd
		close(gui.cmdStopCh)
		gui.cmdStopCh = nil
	}
	gui.cmdMu.Unlock()
	if name != "" {
		gui.logInfo("Cancelled: " + name)
	}
}

// keybindings sets up server mode keybindings
func (gui *ServerGUI) keybindings(g *gocui.Gui) error {
	// Quit
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'q', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}); err != nil {
		return err
	}

	// Cancel running command
	if err := g.SetKeybinding("", gocui.KeyCtrlX, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.cancelCommand()
		return nil
	}); err != nil {
		return err
	}

	// Navigation
	if err := g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, gui.keyDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, gui.keyUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, gui.keyEnter); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, gui.keyBack); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'b', gocui.ModNone, gui.keyBack); err != nil {
		return err
	}

	// Refresh
	if err := g.SetKeybinding("", 'r', gocui.ModNone, gui.keyRefresh); err != nil {
		return err
	}

	// Help
	if err := g.SetKeybinding("", '?', gocui.ModNone, gui.keyHelp); err != nil {
		return err
	}

	// Clear log
	if err := g.SetKeybinding("", 'c', gocui.ModNone, gui.keyClearLog); err != nil {
		return err
	}

	// Scroll
	if err := g.SetKeybinding("", 'j', gocui.ModNone, gui.keyScrollDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'k', gocui.ModNone, gui.keyScrollUp); err != nil {
		return err
	}

	// Confirm dialog keybindings
	if err := g.SetKeybinding(viewServerConfirm, gocui.KeyArrowLeft, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirmLeft()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerConfirm, gocui.KeyArrowRight, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirmRight()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerConfirm, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirmEnter()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerConfirm, gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.closeConfirm()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerConfirm, 'n', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.closeConfirm()
		return nil
	}); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewServerConfirm, 'y', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		gui.confirm.Selected = 0
		gui.confirmEnter()
		return nil
	}); err != nil {
		return err
	}

	// Container actions (in container select screen)
	if err := g.SetKeybinding("", 'l', gocui.ModNone, gui.keyContainerLogs); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 's', gocui.ModNone, gui.keyContainerStop); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'S', gocui.ModNone, gui.keyContainerStart); err != nil {
		return err
	}
	if err := g.SetKeybinding("", 'x', gocui.ModNone, gui.keyContainerRemove); err != nil {
		return err
	}

	return nil
}

func (gui *ServerGUI) keyContainerLogs(g *gocui.Gui, v *gocui.View) error {
	if gui.screen != ServerScreenContainerSelect {
		return nil
	}
	if gui.selectedContainer < len(gui.allContainers) {
		gui.viewContainerLogs(gui.allContainers[gui.selectedContainer])
	}
	return nil
}

func (gui *ServerGUI) keyContainerStop(g *gocui.Gui, v *gocui.View) error {
	if gui.screen != ServerScreenContainerSelect {
		return nil
	}
	if gui.selectedContainer < len(gui.allContainers) {
		ci := gui.allContainers[gui.selectedContainer]
		gui.stopContainer(ci)
	}
	return nil
}

func (gui *ServerGUI) keyContainerStart(g *gocui.Gui, v *gocui.View) error {
	if gui.screen != ServerScreenContainerSelect {
		return nil
	}
	if gui.selectedContainer < len(gui.allContainers) {
		ci := gui.allContainers[gui.selectedContainer]
		gui.startContainer(ci)
	}
	return nil
}

func (gui *ServerGUI) keyContainerRemove(g *gocui.Gui, v *gocui.View) error {
	if gui.screen != ServerScreenContainerSelect {
		return nil
	}
	if gui.selectedContainer < len(gui.allContainers) {
		ci := gui.allContainers[gui.selectedContainer]
		// Only remove stopped containers
		if ci.Container.State != "running" {
			gui.removeContainer(ci)
		} else {
			gui.logError("Cannot remove running container. Stop it first.")
		}
	}
	return nil
}

func (gui *ServerGUI) removeContainer(ci ContainerInfo) {
	gui.showConfirm("Confirm Remove", fmt.Sprintf("Remove container %s?", ci.Container.Name), func() {
		gui.logInfo(fmt.Sprintf("Removing %s...", ci.Container.Name))
		gui.cmdMu.Lock()
		gui.running = true
		gui.runningCmd = "Remove"
		gui.cmdStartTime = time.Now()
		gui.cmdMu.Unlock()

		go func() {
			defer func() {
				gui.cmdMu.Lock()
				gui.running = false
				gui.cmdMu.Unlock()
			}()
			cmd := fmt.Sprintf("docker rm %s", ci.Container.ID)
			if _, err := gui.client.Run(cmd); err != nil {
				gui.logError(fmt.Sprintf("Failed to remove %s: %s", ci.Container.Name, err.Error()))
			} else {
				gui.cmdMu.Lock()
				start := gui.cmdStartTime
				gui.cmdMu.Unlock()
				gui.logSuccess(fmt.Sprintf("Removed %s in %s", ci.Container.Name, formatDuration(time.Since(start))))
				gui.refreshAppsAndContainers()
			}
		}()
	}, nil)
}

// refreshAppsAndContainers refreshes apps from server and rebuilds container list
func (gui *ServerGUI) refreshAppsAndContainers() {
	apps, err := docker.DiscoverApps(gui.client)
	if err != nil {
		gui.logError("Failed to refresh: " + err.Error())
		return
	}
	gui.apps = apps
	// Rebuild container list for current app
	if gui.screen == ServerScreenContainerSelect {
		gui.buildContainerList()
		// Adjust selection if it's now out of bounds
		if gui.selectedContainer >= len(gui.allContainers) {
			gui.selectedContainer = len(gui.allContainers) - 1
			if gui.selectedContainer < 0 {
				gui.selectedContainer = 0
			}
		}
	}
}

func (gui *ServerGUI) keyDown(g *gocui.Gui, v *gocui.View) error {
	switch gui.screen {
	case ServerScreenApps:
		if gui.selectedApp < len(gui.apps)-1 {
			gui.selectedApp++
		}
	case ServerScreenAppMenu:
		// 7 items: Containers, Logs, Details, Actions, Proxy, Exec, Back
		if gui.selectedItem < 6 {
			gui.selectedItem++
		}
	case ServerScreenActionsMenu:
		// 9 items: Boot, Start, Stop, Restart, Remove, Images, Version, Health, Back
		if gui.selectedItem < 8 {
			gui.selectedItem++
		}
	case ServerScreenProxyMenu:
		// 7 items: Logs, Details, Restart, Reboot, Stop, Start, Back
		if gui.selectedItem < 6 {
			gui.selectedItem++
		}
	case ServerScreenContainerSelect:
		if gui.selectedContainer < len(gui.allContainers)-1 {
			gui.selectedContainer++
		}
	}
	return nil
}

func (gui *ServerGUI) keyUp(g *gocui.Gui, v *gocui.View) error {
	switch gui.screen {
	case ServerScreenApps:
		if gui.selectedApp > 0 {
			gui.selectedApp--
		}
	case ServerScreenAppMenu, ServerScreenActionsMenu, ServerScreenProxyMenu:
		if gui.selectedItem > 0 {
			gui.selectedItem--
		}
	case ServerScreenContainerSelect:
		if gui.selectedContainer > 0 {
			gui.selectedContainer--
		}
	}
	return nil
}

func (gui *ServerGUI) keyEnter(g *gocui.Gui, v *gocui.View) error {
	switch gui.screen {
	case ServerScreenApps:
		if len(gui.apps) > 0 {
			gui.screen = ServerScreenAppMenu
			gui.selectedItem = 0
		}
	case ServerScreenAppMenu:
		gui.executeAppMenuAction()
	case ServerScreenActionsMenu:
		gui.executeActionsMenuAction()
	case ServerScreenProxyMenu:
		gui.executeProxyMenuAction()
	case ServerScreenContainerSelect:
		// Enter on container shows its logs by default
		if gui.selectedContainer < len(gui.allContainers) {
			gui.viewContainerLogs(gui.allContainers[gui.selectedContainer])
		}
	case ServerScreenHelp:
		gui.screen = ServerScreenApps
		g.DeleteView(viewHelp)
	}
	return nil
}

func (gui *ServerGUI) keyBack(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ServerScreenConfirm {
		gui.closeConfirm()
		return nil
	}

	// Stop log streaming if active
	gui.streamMu.Lock()
	isStreaming := gui.streamingLogs
	gui.streamMu.Unlock()
	if isStreaming {
		gui.stopLogStream()
		return nil
	}

	switch gui.screen {
	case ServerScreenContainerSelect:
		gui.screen = ServerScreenAppMenu
		gui.selectedContainer = 0
		gui.allContainers = nil
	case ServerScreenActionsMenu, ServerScreenProxyMenu:
		gui.screen = ServerScreenAppMenu
		gui.selectedItem = 0
	case ServerScreenAppMenu:
		gui.screen = ServerScreenApps
		gui.selectedItem = 0
	case ServerScreenHelp:
		gui.screen = ServerScreenApps
		g.DeleteView(viewHelp)
	}
	return nil
}

func (gui *ServerGUI) keyRefresh(g *gocui.Gui, v *gocui.View) error {
	// In container select screen, 'r' restarts the selected container
	if gui.screen == ServerScreenContainerSelect {
		if gui.selectedContainer < len(gui.allContainers) {
			ci := gui.allContainers[gui.selectedContainer]
			gui.restartContainer(ci)
		}
		return nil
	}

	// Otherwise, refresh apps
	gui.logInfo("Refreshing apps...")
	go func() {
		apps, err := docker.DiscoverApps(gui.client)
		if err != nil {
			gui.logError("Failed to refresh: " + err.Error())
			return
		}
		gui.apps = apps
		gui.logSuccess(fmt.Sprintf("Found %d app(s)", len(apps)))
	}()
	return nil
}

func (gui *ServerGUI) restartContainer(ci ContainerInfo) {
	gui.logInfo(fmt.Sprintf("Restarting %s...", ci.Container.Name))
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Restart"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		if err := docker.RestartContainer(gui.client, ci.Container.ID); err != nil {
			gui.logError(fmt.Sprintf("Failed to restart %s: %s", ci.Container.Name, err.Error()))
		} else {
			gui.logSuccess(fmt.Sprintf("Restarted %s", ci.Container.Name))
		}
	}()
}

func (gui *ServerGUI) keyHelp(g *gocui.Gui, v *gocui.View) error {
	if gui.screen == ServerScreenHelp {
		gui.screen = ServerScreenApps
		g.DeleteView(viewHelp)
	} else {
		gui.screen = ServerScreenHelp
	}
	return nil
}

func (gui *ServerGUI) keyClearLog(g *gocui.Gui, v *gocui.View) error {
	gui.logMu.Lock()
	gui.logLines = make([]string, 0, 1000)
	gui.logMu.Unlock()
	gui.logScroll = 0
	return nil
}

func (gui *ServerGUI) keyScrollDown(g *gocui.Gui, v *gocui.View) error {
	gui.logScroll += 5
	return nil
}

func (gui *ServerGUI) keyScrollUp(g *gocui.Gui, v *gocui.View) error {
	if gui.logScroll > 0 {
		gui.logScroll -= 5
		if gui.logScroll < 0 {
			gui.logScroll = 0
		}
	}
	return nil
}

// executeAppMenuAction handles main app menu selections
func (gui *ServerGUI) executeAppMenuAction() {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]

	// Main menu: 0: Containers, 1: Logs, 2: Details, 3: Actions→, 4: Proxy→, 5: Exec, 6: Back
	switch gui.selectedItem {
	case 0: // Containers →
		gui.screen = ServerScreenContainerSelect
		gui.selectedContainer = 0
		gui.buildContainerList()
	case 1: // Logs (live)
		gui.viewAppLogs(app)
	case 2: // Details
		gui.showAppDetails(app)
	case 3: // Actions →
		gui.screen = ServerScreenActionsMenu
		gui.selectedItem = 0
	case 4: // Proxy →
		gui.screen = ServerScreenProxyMenu
		gui.selectedItem = 0
	case 5: // Exec (shell)
		gui.execShell(app)
	case 6: // Back
		gui.screen = ServerScreenApps
		gui.selectedItem = 0
	}
}

// executeActionsMenuAction handles actions submenu selections
func (gui *ServerGUI) executeActionsMenuAction() {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]

	// Actions menu: 0: Boot, 1: Start, 2: Stop, 3: Restart, 4: Remove, 5: Images, 6: Version, 7: Health, 8: Back
	switch gui.selectedItem {
	case 0: // Boot / Reboot
		gui.rebootApp(app)
	case 1: // Start
		gui.startApp(app)
	case 2: // Stop
		gui.stopApp(app)
	case 3: // Restart
		gui.restartApp(app)
	case 4: // Remove stopped
		gui.removeStoppedContainers(app)
	case 5: // Images
		gui.showAppImages(app)
	case 6: // Version
		gui.showAppVersion(app)
	case 7: // Health
		gui.showAppHealth(app)
	case 8: // Back
		gui.screen = ServerScreenAppMenu
		gui.selectedItem = 0
	}
}

// executeProxyMenuAction handles proxy submenu selections
func (gui *ServerGUI) executeProxyMenuAction() {
	// Proxy menu: 0: Logs, 1: Details, 2: Restart, 3: Reboot, 4: Stop, 5: Start, 6: Back
	switch gui.selectedItem {
	case 0: // Logs (live)
		gui.viewProxyLogs()
	case 1: // Details
		gui.showProxyDetails()
	case 2: // Restart
		gui.proxyRestart()
	case 3: // Reboot
		gui.proxyReboot()
	case 4: // Stop
		gui.proxyStop()
	case 5: // Start
		gui.proxyStart()
	case 6: // Back
		gui.screen = ServerScreenAppMenu
		gui.selectedItem = 0
	}
}

func (gui *ServerGUI) viewAppLogs(app docker.App) {
	// View logs from all containers (web + accessories)
	allContainers := app.Containers
	for _, acc := range app.Accessories {
		allContainers = append(allContainers, acc.Containers...)
	}

	if len(allContainers) == 0 {
		gui.logError("No containers to view logs from")
		return
	}

	gui.logInfo(fmt.Sprintf("Fetching logs from %d container(s)...", len(allContainers)))

	go func() {
		for _, container := range allContainers {
			output, err := docker.GetContainerLogs(gui.client, container.ID, 50, false)
			if err != nil {
				gui.logError(fmt.Sprintf("Failed to get logs from %s: %s", container.Name, err.Error()))
				continue
			}

			gui.appendLog([]string{fmt.Sprintf("─── %s ───", container.Name)})
			lines := splitLines(output)
			gui.appendLog(lines)
		}
		gui.logSuccess("Fetched logs from all containers")
	}()
}

func (gui *ServerGUI) viewContainerLogs(ci ContainerInfo) {
	// Stop any existing stream
	gui.stopLogStream()

	gui.logInfo(fmt.Sprintf("Streaming logs from %s [%s]... (press Esc to stop)", ci.Container.Name, ci.Role))

	gui.streamMu.Lock()
	gui.streamingLogs = true
	gui.streamingContainer = ci.Container.Name
	gui.liveLogsStop = make(chan struct{})
	stopCh := gui.liveLogsStop
	gui.streamMu.Unlock()

	go func() {
		lastUpdate := time.Now()
		throttle := 80 * time.Millisecond
		err := docker.StreamContainerLogs(gui.client, ci.Container.ID, func(line string) {
			gui.appendLog([]string{line})
			if time.Since(lastUpdate) < throttle {
				return
			}
			lastUpdate = time.Now()
			gui.g.Update(func(g *gocui.Gui) error { return nil })
		}, stopCh)

		gui.streamMu.Lock()
		gui.streamingLogs = false
		gui.streamMu.Unlock()
		if err != nil {
			gui.logError("Log stream ended: " + err.Error())
		} else {
			gui.logInfo("Log stream stopped")
		}
	}()
}

func (gui *ServerGUI) stopLogStream() {
	gui.streamMu.Lock()
	defer gui.streamMu.Unlock()
	if gui.streamingLogs && gui.liveLogsStop != nil {
		close(gui.liveLogsStop)
		gui.liveLogsStop = nil
		gui.streamingLogs = false
	}
}

func (gui *ServerGUI) stopContainer(ci ContainerInfo) {
	gui.showConfirm("Confirm Stop", fmt.Sprintf("Stop container %s?", ci.Container.Name), func() {
		gui.logInfo(fmt.Sprintf("Stopping %s...", ci.Container.Name))
		gui.cmdMu.Lock()
		gui.running = true
		gui.runningCmd = "Stop"
		gui.cmdStartTime = time.Now()
		gui.cmdMu.Unlock()

		go func() {
			defer func() {
				gui.cmdMu.Lock()
				gui.running = false
				gui.cmdMu.Unlock()
			}()
			if err := docker.StopContainer(gui.client, ci.Container.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to stop %s: %s", ci.Container.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Stopped %s", ci.Container.Name))
			}
		}()
	}, nil)
}

func (gui *ServerGUI) startContainer(ci ContainerInfo) {
	gui.logInfo(fmt.Sprintf("Starting %s...", ci.Container.Name))
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Start"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		if err := docker.StartContainer(gui.client, ci.Container.ID); err != nil {
			gui.logError(fmt.Sprintf("Failed to start %s: %s", ci.Container.Name, err.Error()))
		} else {
			gui.logSuccess(fmt.Sprintf("Started %s", ci.Container.Name))
		}
	}()
}

func (gui *ServerGUI) restartApp(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to restart")
		return
	}

	gui.logInfo(fmt.Sprintf("Restarting %s...", app.Service))
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Restart"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		for _, c := range app.Containers {
			if err := docker.RestartContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to restart %s: %s", c.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Restarted %s", c.Name))
			}
		}
		gui.cmdMu.Lock()
		start := gui.cmdStartTime
		gui.cmdMu.Unlock()
		gui.logSuccess(fmt.Sprintf("Restart completed in %s", formatDuration(time.Since(start))))
	}()
}

func (gui *ServerGUI) stopApp(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to stop")
		return
	}

	gui.showConfirm("Confirm Stop", fmt.Sprintf("Stop all containers for %s?", app.Service), func() {
		gui.logInfo(fmt.Sprintf("Stopping %s...", app.Service))
		gui.cmdMu.Lock()
		gui.running = true
		gui.runningCmd = "Stop"
		gui.cmdStartTime = time.Now()
		gui.cmdMu.Unlock()

		go func() {
			defer func() {
				gui.cmdMu.Lock()
				gui.running = false
				gui.cmdMu.Unlock()
			}()
			for _, c := range app.Containers {
				if err := docker.StopContainer(gui.client, c.ID); err != nil {
					gui.logError(fmt.Sprintf("Failed to stop %s: %s", c.Name, err.Error()))
				} else {
					gui.logSuccess(fmt.Sprintf("Stopped %s", c.Name))
				}
			}
			gui.cmdMu.Lock()
			start := gui.cmdStartTime
			gui.cmdMu.Unlock()
			gui.logSuccess(fmt.Sprintf("Stop completed in %s", formatDuration(time.Since(start))))
		}()
	}, nil)
}

func (gui *ServerGUI) startApp(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to start")
		return
	}

	gui.logInfo(fmt.Sprintf("Starting %s...", app.Service))
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Start"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		for _, c := range app.Containers {
			if err := docker.StartContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to start %s: %s", c.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Started %s", c.Name))
			}
		}
		gui.cmdMu.Lock()
		start := gui.cmdStartTime
		gui.cmdMu.Unlock()
		gui.logSuccess(fmt.Sprintf("Start completed in %s", formatDuration(time.Since(start))))
	}()
}

func (gui *ServerGUI) showAppDetails(app docker.App) {
	gui.logInfo(fmt.Sprintf("=== %s Details ===", app.Service))

	go func() {
		// Get all container IDs
		allContainers := app.Containers
		for _, acc := range app.Accessories {
			allContainers = append(allContainers, acc.Containers...)
		}

		for _, c := range allContainers {
			// Get container inspect details
			cmd := fmt.Sprintf("docker inspect --format '{{.State.Status}} | Started: {{.State.StartedAt}} | Image: {{.Config.Image}}' %s", c.ID)
			output, err := gui.client.Run(cmd)
			if err != nil {
				gui.appendLog([]string{fmt.Sprintf("  %s: error - %s", c.Name, err.Error())})
				continue
			}
			gui.appendLog([]string{fmt.Sprintf("  %s: %s", c.Name, strings.TrimSpace(output))})
		}
		gui.logSuccess("Details fetched")
	}()
}

func (gui *ServerGUI) rebootApp(app docker.App) {
	gui.logInfo(fmt.Sprintf("Rebooting %s (stop + start)...", app.Service))
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Reboot"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		// Stop all containers
		for _, c := range app.Containers {
			if err := docker.StopContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to stop %s: %s", c.Name, err.Error()))
			}
		}
		// Start all containers
		for _, c := range app.Containers {
			if err := docker.StartContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to start %s: %s", c.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Rebooted %s", c.Name))
			}
		}
		gui.cmdMu.Lock()
		start := gui.cmdStartTime
		gui.cmdMu.Unlock()
		gui.logSuccess(fmt.Sprintf("Reboot completed in %s", formatDuration(time.Since(start))))
	}()
}

func (gui *ServerGUI) execShell(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers available")
		return
	}

	container := app.Containers[0]
	gui.logInfo(fmt.Sprintf("Opening shell in %s...", container.Name))
	gui.logInfo("Running: docker exec -it ... /bin/sh")

	go func() {
		// Try common shells
		shells := []string{"/bin/bash", "/bin/sh"}
		for _, shell := range shells {
			cmd := fmt.Sprintf("docker exec %s which %s 2>/dev/null", container.ID, shell)
			if output, err := gui.client.Run(cmd); err == nil && strings.TrimSpace(output) != "" {
				gui.logInfo(fmt.Sprintf("Shell available: %s", shell))
				gui.logInfo("To connect manually run:")
				gui.logInfo(fmt.Sprintf("  ssh %s docker exec -it %s %s", gui.client.HostDisplay(), container.Name, shell))
				return
			}
		}
		gui.logError("No shell found in container")
	}()
}

func (gui *ServerGUI) viewProxyLogs() {
	gui.stopLogStream()
	gui.logInfo("Streaming kamal-proxy logs... (press Esc to stop)")

	gui.streamMu.Lock()
	gui.streamingLogs = true
	gui.streamingContainer = "kamal-proxy"
	gui.liveLogsStop = make(chan struct{})
	stopCh := gui.liveLogsStop
	gui.streamMu.Unlock()

	go func() {
		// Find kamal-proxy container
		cmd := `docker ps --filter "name=kamal-proxy" --format "{{.ID}}" | head -1`
		proxyID, err := gui.client.Run(cmd)
		if err != nil || strings.TrimSpace(proxyID) == "" {
			gui.logError("kamal-proxy container not found")
			gui.streamMu.Lock()
			gui.streamingLogs = false
			gui.streamMu.Unlock()
			return
		}

		proxyID = strings.TrimSpace(proxyID)
		lastUpdate := time.Now()
		throttle := 80 * time.Millisecond
		err = docker.StreamContainerLogs(gui.client, proxyID, func(line string) {
			gui.appendLog([]string{line})
			if time.Since(lastUpdate) < throttle {
				return
			}
			lastUpdate = time.Now()
			gui.g.Update(func(g *gocui.Gui) error { return nil })
		}, stopCh)

		gui.streamMu.Lock()
		gui.streamingLogs = false
		gui.streamMu.Unlock()
		if err != nil {
			gui.logError("Proxy log stream ended: " + err.Error())
		} else {
			gui.logInfo("Proxy log stream stopped")
		}
	}()
}

func (gui *ServerGUI) showProxyDetails() {
	gui.logInfo("=== kamal-proxy Details ===")

	go func() {
		cmd := `docker ps --filter "name=kamal-proxy" --format "Name: {{.Names}}\nImage: {{.Image}}\nStatus: {{.Status}}\nPorts: {{.Ports}}"`
		output, err := gui.client.Run(cmd)
		if err != nil {
			gui.logError("Failed to get proxy details: " + err.Error())
			return
		}

		if strings.TrimSpace(output) == "" {
			gui.logError("kamal-proxy container not found")
			return
		}

		for _, line := range strings.Split(output, "\n") {
			if line != "" {
				gui.appendLog([]string{"  " + line})
			}
		}
		gui.logSuccess("Proxy details fetched")
	}()
}

// --- New App Commands ---

func (gui *ServerGUI) showAppImages(app docker.App) {
	gui.logInfo(fmt.Sprintf("=== %s Images ===", app.Service))

	go func() {
		allContainers := app.Containers
		for _, acc := range app.Accessories {
			allContainers = append(allContainers, acc.Containers...)
		}

		// Collect unique images
		images := make(map[string]bool)
		for _, c := range allContainers {
			images[c.Image] = true
		}

		for image := range images {
			// Get image details
			cmd := fmt.Sprintf("docker images --format 'ID: {{.ID}} | Size: {{.Size}} | Created: {{.CreatedSince}}' %s", image)
			output, err := gui.client.Run(cmd)
			if err != nil {
				gui.appendLog([]string{fmt.Sprintf("  %s: error - %s", image, err.Error())})
				continue
			}
			gui.appendLog([]string{fmt.Sprintf("  %s", image)})
			gui.appendLog([]string{fmt.Sprintf("    %s", strings.TrimSpace(output))})
		}
		gui.logSuccess("Images fetched")
	}()
}

func (gui *ServerGUI) showAppVersion(app docker.App) {
	gui.logInfo(fmt.Sprintf("=== %s Version ===", app.Service))

	version := docker.GetAppVersion(app.Containers)
	gui.appendLog([]string{fmt.Sprintf("  Service: %s", app.Service)})
	gui.appendLog([]string{fmt.Sprintf("  Destination: %s", app.Destination)})
	gui.appendLog([]string{fmt.Sprintf("  Version: %s", version)})

	// Show version from labels if available
	if len(app.Containers) > 0 {
		c := app.Containers[0]
		if v, ok := c.Labels["version"]; ok {
			gui.appendLog([]string{fmt.Sprintf("  Label version: %s", v)})
		}
		gui.appendLog([]string{fmt.Sprintf("  Image: %s", c.Image)})
	}
}

func (gui *ServerGUI) showAppHealth(app docker.App) {
	gui.logInfo(fmt.Sprintf("=== %s Health ===", app.Service))

	go func() {
		allContainers := app.Containers
		for _, acc := range app.Accessories {
			allContainers = append(allContainers, acc.Containers...)
		}

		for _, c := range allContainers {
			// Check container health status
			cmd := fmt.Sprintf("docker inspect --format '{{.State.Status}} | Health: {{if .State.Health}}{{.State.Health.Status}}{{else}}no healthcheck{{end}} | Restarts: {{.RestartCount}}' %s", c.ID)
			output, err := gui.client.Run(cmd)
			if err != nil {
				gui.appendLog([]string{fmt.Sprintf("  %s: error - %s", c.Name, err.Error())})
				continue
			}

			status := red("●")
			if c.State == "running" {
				status = green("●")
			}
			gui.appendLog([]string{fmt.Sprintf("  %s %s: %s", status, c.Name, strings.TrimSpace(output))})
		}
		gui.logSuccess("Health check completed")
	}()
}

func (gui *ServerGUI) removeStoppedContainers(app docker.App) {
	gui.logInfo(fmt.Sprintf("Removing stopped containers for %s...", app.Service))
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Remove"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		removed := 0
		allContainers := app.Containers
		for _, acc := range app.Accessories {
			allContainers = append(allContainers, acc.Containers...)
		}

		for _, c := range allContainers {
			if c.State != "running" {
				cmd := fmt.Sprintf("docker rm %s", c.ID)
				if _, err := gui.client.Run(cmd); err != nil {
					gui.logError(fmt.Sprintf("Failed to remove %s: %s", c.Name, err.Error()))
				} else {
					gui.logSuccess(fmt.Sprintf("Removed %s", c.Name))
					removed++
				}
			}
		}

		if removed == 0 {
			gui.logInfo("No stopped containers to remove")
		} else {
			gui.cmdMu.Lock()
			start := gui.cmdStartTime
			gui.cmdMu.Unlock()
			gui.logSuccess(fmt.Sprintf("Removed %d container(s) in %s", removed, formatDuration(time.Since(start))))
			gui.refreshAppsAndContainers()
		}
	}()
}

// --- Proxy Management ---

func (gui *ServerGUI) getProxyContainerID() (string, error) {
	cmd := `docker ps -a --filter "name=kamal-proxy" --format "{{.ID}}" | head -1`
	output, err := gui.client.Run(cmd)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(output)
	if id == "" {
		return "", fmt.Errorf("kamal-proxy container not found")
	}
	return id, nil
}

func (gui *ServerGUI) proxyRestart() {
	gui.logInfo("Restarting kamal-proxy...")
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Proxy Restart"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		proxyID, err := gui.getProxyContainerID()
		if err != nil {
			gui.logError(err.Error())
			return
		}

		if err := docker.RestartContainer(gui.client, proxyID); err != nil {
			gui.logError(fmt.Sprintf("Failed to restart proxy: %s", err.Error()))
		} else {
			gui.cmdMu.Lock()
			start := gui.cmdStartTime
			gui.cmdMu.Unlock()
			gui.logSuccess(fmt.Sprintf("Proxy restarted in %s", formatDuration(time.Since(start))))
		}
	}()
}

func (gui *ServerGUI) proxyReboot() {
	gui.logInfo("Rebooting kamal-proxy (stop + start)...")
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Proxy Reboot"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		proxyID, err := gui.getProxyContainerID()
		if err != nil {
			gui.logError(err.Error())
			return
		}

		if err := docker.StopContainer(gui.client, proxyID); err != nil {
			gui.logError(fmt.Sprintf("Failed to stop proxy: %s", err.Error()))
		}

		if err := docker.StartContainer(gui.client, proxyID); err != nil {
			gui.logError(fmt.Sprintf("Failed to start proxy: %s", err.Error()))
		} else {
			gui.cmdMu.Lock()
			start := gui.cmdStartTime
			gui.cmdMu.Unlock()
			gui.logSuccess(fmt.Sprintf("Proxy rebooted in %s", formatDuration(time.Since(start))))
		}
	}()
}

func (gui *ServerGUI) proxyStop() {
	gui.showConfirm("Confirm Proxy Stop", "Stop kamal-proxy?", func() {
		gui.logInfo("Stopping kamal-proxy...")
		gui.cmdMu.Lock()
		gui.running = true
		gui.runningCmd = "Proxy Stop"
		gui.cmdStartTime = time.Now()
		gui.cmdMu.Unlock()

		go func() {
			defer func() {
				gui.cmdMu.Lock()
				gui.running = false
				gui.cmdMu.Unlock()
			}()
			proxyID, err := gui.getProxyContainerID()
			if err != nil {
				gui.logError(err.Error())
				return
			}

			if err := docker.StopContainer(gui.client, proxyID); err != nil {
				gui.logError(fmt.Sprintf("Failed to stop proxy: %s", err.Error()))
			} else {
				gui.cmdMu.Lock()
				start := gui.cmdStartTime
				gui.cmdMu.Unlock()
				gui.logSuccess(fmt.Sprintf("Proxy stopped in %s", formatDuration(time.Since(start))))
			}
		}()
	}, nil)
}

func (gui *ServerGUI) proxyStart() {
	gui.logInfo("Starting kamal-proxy...")
	gui.cmdMu.Lock()
	gui.running = true
	gui.runningCmd = "Proxy Start"
	gui.cmdStartTime = time.Now()
	gui.cmdMu.Unlock()

	go func() {
		defer func() {
			gui.cmdMu.Lock()
			gui.running = false
			gui.cmdMu.Unlock()
		}()
		proxyID, err := gui.getProxyContainerID()
		if err != nil {
			gui.logError(err.Error())
			return
		}

		if err := docker.StartContainer(gui.client, proxyID); err != nil {
			gui.logError(fmt.Sprintf("Failed to start proxy: %s", err.Error()))
		} else {
			gui.cmdMu.Lock()
			start := gui.cmdStartTime
			gui.cmdMu.Unlock()
			gui.logSuccess(fmt.Sprintf("Proxy started in %s", formatDuration(time.Since(start))))
		}
	}()
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
