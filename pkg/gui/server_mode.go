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

// ServerGUI holds server mode TUI state
type ServerGUI struct {
	g            *gocui.Gui
	version      string
	host         string
	client       *ssh.Client
	apps         []docker.App
	selectedApp  int
	selectedItem int // For submenu navigation
	screen       ServerScreen
	logLines     []string
	logMu        sync.Mutex
	logScroll    int
	running      bool
	runningCmd   string
	cmdStartTime time.Time
	spinner      *Spinner
}

// ServerScreen represents the current screen in server mode
type ServerScreen int

const (
	ServerScreenApps ServerScreen = iota
	ServerScreenAppMenu
	ServerScreenContainers
	ServerScreenAccessories
	ServerScreenHelp
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
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
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

	status := green("✓ Connected")
	if gui.running {
		elapsed := time.Since(gui.cmdStartTime)
		status = yellow(gui.spinner.Frame()) + " " + gui.runningCmd + " " + dim(formatDuration(elapsed))
	}

	breadcrumb := gui.getBreadcrumb()
	fmt.Fprintf(v, " %s%s %s | %s | %s",
		iconRocket, bold("Lazykamal"), dim(gui.version),
		breadcrumb,
		status)
}

func (gui *ServerGUI) getBreadcrumb() string {
	parts := []string{cyan(gui.client.HostDisplay())}

	if gui.screen == ServerScreenAppMenu && gui.selectedApp < len(gui.apps) {
		app := gui.apps[gui.selectedApp]
		parts = append(parts, fmt.Sprintf("%s (%s)", app.Service, app.Destination))
	}

	return dim(fmt.Sprintf("Server: %s", parts[0]))
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

	menuItems := []string{
		"View Logs",
		"View Containers",
		"Restart App",
		"Stop App",
		"Start App",
		"───────────────",
		"View Accessories",
		"───────────────",
		"Back",
	}

	for i, item := range menuItems {
		if item == "───────────────" {
			fmt.Fprintln(v, dim("  "+item))
			continue
		}

		prefix := "  "
		if i == gui.selectedItem {
			prefix = cyan(iconArrow) + " "
		}

		// Color destructive actions
		if item == "Stop App" {
			item = red(item)
		}

		fmt.Fprintln(v, prefix+item)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" Enter: select  b/Esc: back"))
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
	width := 50
	height := 18
	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2

	if v, err := g.SetView(viewHelp, x0, y0, x0+width, y0+height); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Server Mode Help "
	}

	v, _ := g.View(viewHelp)
	if v == nil {
		return nil
	}
	v.Clear()

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " KEYBOARD SHORTCUTS")
	fmt.Fprintln(v, " ══════════════════════════════════════")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, "   ↑/↓       Navigate apps/menu")
	fmt.Fprintln(v, "   Enter     Select / Execute")
	fmt.Fprintln(v, "   b/Esc     Go back")
	fmt.Fprintln(v, "   r         Refresh apps")
	fmt.Fprintln(v, "   j/k       Scroll log down/up")
	fmt.Fprintln(v, "   c         Clear log")
	fmt.Fprintln(v, "   ?         Toggle help")
	fmt.Fprintln(v, "   q         Quit")
	fmt.Fprintln(v, "")
	fmt.Fprintln(v, " ══════════════════════════════════════")
	fmt.Fprintln(v, "   Press ? or Esc to close")

	return nil
}

func (gui *ServerGUI) appendLog(lines []string) {
	gui.logMu.Lock()
	defer gui.logMu.Unlock()
	for _, line := range lines {
		gui.logLines = append(gui.logLines, timestampedLine(line))
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

	return nil
}

func (gui *ServerGUI) keyDown(g *gocui.Gui, v *gocui.View) error {
	switch gui.screen {
	case ServerScreenApps:
		if gui.selectedApp < len(gui.apps)-1 {
			gui.selectedApp++
		}
	case ServerScreenAppMenu:
		gui.selectedItem++
		// Skip separator lines
		if gui.selectedItem == 5 || gui.selectedItem == 7 {
			gui.selectedItem++
		}
		if gui.selectedItem > 8 {
			gui.selectedItem = 8
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
	case ServerScreenAppMenu:
		gui.selectedItem--
		// Skip separator lines
		if gui.selectedItem == 5 || gui.selectedItem == 7 {
			gui.selectedItem--
		}
		if gui.selectedItem < 0 {
			gui.selectedItem = 0
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
		gui.executeAppAction()
	case ServerScreenHelp:
		gui.screen = ServerScreenApps
		g.DeleteView(viewHelp)
	}
	return nil
}

func (gui *ServerGUI) keyBack(g *gocui.Gui, v *gocui.View) error {
	switch gui.screen {
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

func (gui *ServerGUI) executeAppAction() {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]

	switch gui.selectedItem {
	case 0: // View Logs
		gui.viewAppLogs(app)
	case 1: // View Containers
		gui.viewContainers(app)
	case 2: // Restart App
		gui.restartApp(app)
	case 3: // Stop App
		gui.stopApp(app)
	case 4: // Start App
		gui.startApp(app)
	case 6: // View Accessories
		gui.viewAccessories(app)
	case 8: // Back
		gui.screen = ServerScreenApps
		gui.selectedItem = 0
	}
}

func (gui *ServerGUI) viewAppLogs(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to view logs from")
		return
	}

	container := app.Containers[0]
	gui.logInfo(fmt.Sprintf("Fetching logs for %s...", container.Name))

	go func() {
		output, err := docker.GetContainerLogs(gui.client, container.ID, 100, false)
		if err != nil {
			gui.logError("Failed to get logs: " + err.Error())
			return
		}

		lines := splitLines(output)
		gui.appendLog(lines)
		gui.logSuccess(fmt.Sprintf("Fetched %d log lines", len(lines)))
	}()
}

func (gui *ServerGUI) viewContainers(app docker.App) {
	gui.logInfo(fmt.Sprintf("Containers for %s:", app.Service))
	for _, c := range app.Containers {
		status := "running"
		if c.State != "running" {
			status = c.State
		}
		gui.appendLog([]string{fmt.Sprintf("  %s - %s - %s", c.Name, c.ID[:12], status)})
	}
}

func (gui *ServerGUI) restartApp(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to restart")
		return
	}

	gui.logInfo(fmt.Sprintf("Restarting %s...", app.Service))
	gui.running = true
	gui.runningCmd = "Restart"
	gui.cmdStartTime = time.Now()

	go func() {
		for _, c := range app.Containers {
			if err := docker.RestartContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to restart %s: %s", c.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Restarted %s", c.Name))
			}
		}
		gui.running = false
		gui.logSuccess(fmt.Sprintf("Restart completed in %s", formatDuration(time.Since(gui.cmdStartTime))))
	}()
}

func (gui *ServerGUI) stopApp(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to stop")
		return
	}

	gui.logInfo(fmt.Sprintf("Stopping %s...", app.Service))
	gui.running = true
	gui.runningCmd = "Stop"
	gui.cmdStartTime = time.Now()

	go func() {
		for _, c := range app.Containers {
			if err := docker.StopContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to stop %s: %s", c.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Stopped %s", c.Name))
			}
		}
		gui.running = false
		gui.logSuccess(fmt.Sprintf("Stop completed in %s", formatDuration(time.Since(gui.cmdStartTime))))
	}()
}

func (gui *ServerGUI) startApp(app docker.App) {
	if len(app.Containers) == 0 {
		gui.logError("No containers to start")
		return
	}

	gui.logInfo(fmt.Sprintf("Starting %s...", app.Service))
	gui.running = true
	gui.runningCmd = "Start"
	gui.cmdStartTime = time.Now()

	go func() {
		for _, c := range app.Containers {
			if err := docker.StartContainer(gui.client, c.ID); err != nil {
				gui.logError(fmt.Sprintf("Failed to start %s: %s", c.Name, err.Error()))
			} else {
				gui.logSuccess(fmt.Sprintf("Started %s", c.Name))
			}
		}
		gui.running = false
		gui.logSuccess(fmt.Sprintf("Start completed in %s", formatDuration(time.Since(gui.cmdStartTime))))
	}()
}

func (gui *ServerGUI) viewAccessories(app docker.App) {
	if len(app.Accessories) == 0 {
		gui.logInfo("No accessories for this app")
		return
	}

	gui.logInfo(fmt.Sprintf("Accessories for %s:", app.Service))
	for _, acc := range app.Accessories {
		gui.appendLog([]string{fmt.Sprintf("  %s:", acc.Name)})
		for _, c := range acc.Containers {
			status := "running"
			if c.State != "running" {
				status = c.State
			}
			gui.appendLog([]string{fmt.Sprintf("    - %s (%s)", c.Name, status)})
		}
	}
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
