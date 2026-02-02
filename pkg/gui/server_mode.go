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
	// Live log streaming
	streamingLogs      bool
	liveLogsStop       chan struct{}
	streamingContainer string
}

// ServerScreen represents the current screen in server mode
type ServerScreen int

const (
	ServerScreenApps ServerScreen = iota
	ServerScreenAppMenu
	ServerScreenContainerSelect // New: select a container for actions
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
	if gui.streamingLogs {
		status = cyan(gui.spinner.Frame()) + " Streaming logs " + dim("(Esc to stop)")
	} else if gui.running {
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
	case ServerScreenContainerSelect:
		gui.renderContainerSelect(v)
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

	// Menu structure mirrors Kamal commands
	// 0: Containers, 1: sep, 2: Logs, 3: Details, 4: sep, 5: Boot, 6: Start, 7: Stop, 8: Restart
	// 9: sep, 10: Exec, 11: sep, 12: Proxy, 13: sep, 14: Back
	menuItems := []string{
		"Containers...",    // 0 - Select and manage individual containers
		"─── App ───",      // 1 - separator
		"Logs (streaming)", // 2 - Live logs
		"Details",          // 3 - Show container details
		"─── Actions ───",  // 4 - separator
		"Boot / Reboot",    // 5 - Restart all containers
		"Start",            // 6 - Start all containers
		"Stop",             // 7 - Stop all containers
		"Restart",          // 8 - Restart all containers
		"─── Commands ───", // 9 - separator
		"Exec (shell)",     // 10 - Execute shell in container
		"─── Proxy ───",    // 11 - separator
		"Proxy Logs",       // 12 - View proxy logs
		"Proxy Details",    // 13 - Proxy container details
		"───────────────", // 14 - separator
		"Back", // 15 - Go back
	}

	// Track actual selectable items (skip separators)
	selectableIdx := 0
	for _, item := range menuItems {
		if strings.HasPrefix(item, "───") {
			fmt.Fprintln(v, dim("  "+item))
			continue
		}

		prefix := "  "
		if selectableIdx == gui.selectedItem {
			prefix = cyan(iconArrow) + " "
		}
		selectableIdx++

		// Color destructive actions
		displayItem := item
		if item == "Stop" {
			displayItem = red(item)
		}

		fmt.Fprintln(v, prefix+displayItem)
	}

	fmt.Fprintln(v, "")
	fmt.Fprintln(v, dim(" Enter: select  b/Esc: back"))
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
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, dim(" Actions:"))
		fmt.Fprintln(v, "   l - View Logs")
		fmt.Fprintln(v, "   r - Restart")
		fmt.Fprintln(v, "   s - Stop")
		fmt.Fprintln(v, "   S - Start")
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
	if gui.streamingLogs {
		v.Title = fmt.Sprintf(" LIVE: %s (Esc to stop) ", truncate(gui.streamingContainer, 20))
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

func (gui *ServerGUI) keyDown(g *gocui.Gui, v *gocui.View) error {
	switch gui.screen {
	case ServerScreenApps:
		if gui.selectedApp < len(gui.apps)-1 {
			gui.selectedApp++
		}
	case ServerScreenAppMenu:
		// 11 selectable items (0-10): Containers, Logs, Details, Boot, Start, Stop, Restart, Exec, ProxyLogs, ProxyDetails, Back
		if gui.selectedItem < 10 {
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
	case ServerScreenAppMenu:
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
		gui.executeAppAction()
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
	// Stop log streaming if active
	if gui.streamingLogs {
		gui.stopLogStream()
		return nil
	}

	switch gui.screen {
	case ServerScreenContainerSelect:
		gui.screen = ServerScreenAppMenu
		gui.selectedContainer = 0
		gui.allContainers = nil
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
	gui.running = true
	gui.runningCmd = "Restart"
	gui.cmdStartTime = time.Now()

	go func() {
		if err := docker.RestartContainer(gui.client, ci.Container.ID); err != nil {
			gui.logError(fmt.Sprintf("Failed to restart %s: %s", ci.Container.Name, err.Error()))
		} else {
			gui.logSuccess(fmt.Sprintf("Restarted %s", ci.Container.Name))
		}
		gui.running = false
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

func (gui *ServerGUI) executeAppAction() {
	if gui.selectedApp >= len(gui.apps) {
		return
	}
	app := gui.apps[gui.selectedApp]

	// Selectable menu items (excluding separators):
	// 0: Containers..., 1: Logs, 2: Details, 3: Boot/Reboot, 4: Start, 5: Stop, 6: Restart
	// 7: Exec, 8: Proxy Logs, 9: Proxy Details, 10: Back
	switch gui.selectedItem {
	case 0: // Containers...
		gui.screen = ServerScreenContainerSelect
		gui.selectedContainer = 0
		gui.buildContainerList()
	case 1: // Logs (streaming)
		gui.viewAppLogs(app)
	case 2: // Details
		gui.showAppDetails(app)
	case 3: // Boot / Reboot
		gui.rebootApp(app)
	case 4: // Start
		gui.startApp(app)
	case 5: // Stop
		gui.stopApp(app)
	case 6: // Restart
		gui.restartApp(app)
	case 7: // Exec (shell)
		gui.execShell(app)
	case 8: // Proxy Logs
		gui.viewProxyLogs()
	case 9: // Proxy Details
		gui.showProxyDetails()
	case 10: // Back
		gui.screen = ServerScreenApps
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

	gui.streamingLogs = true
	gui.streamingContainer = ci.Container.Name
	gui.liveLogsStop = make(chan struct{})

	go func() {
		err := docker.StreamContainerLogs(gui.client, ci.Container.ID, func(line string) {
			gui.appendLog([]string{line})
			// Trigger UI update
			gui.g.Update(func(g *gocui.Gui) error { return nil })
		}, gui.liveLogsStop)

		gui.streamingLogs = false
		if err != nil {
			gui.logError("Log stream ended: " + err.Error())
		} else {
			gui.logInfo("Log stream stopped")
		}
	}()
}

func (gui *ServerGUI) stopLogStream() {
	if gui.streamingLogs && gui.liveLogsStop != nil {
		close(gui.liveLogsStop)
		gui.streamingLogs = false
	}
}

func (gui *ServerGUI) stopContainer(ci ContainerInfo) {
	gui.logInfo(fmt.Sprintf("Stopping %s...", ci.Container.Name))
	gui.running = true
	gui.runningCmd = "Stop"
	gui.cmdStartTime = time.Now()

	go func() {
		if err := docker.StopContainer(gui.client, ci.Container.ID); err != nil {
			gui.logError(fmt.Sprintf("Failed to stop %s: %s", ci.Container.Name, err.Error()))
		} else {
			gui.logSuccess(fmt.Sprintf("Stopped %s", ci.Container.Name))
		}
		gui.running = false
	}()
}

func (gui *ServerGUI) startContainer(ci ContainerInfo) {
	gui.logInfo(fmt.Sprintf("Starting %s...", ci.Container.Name))
	gui.running = true
	gui.runningCmd = "Start"
	gui.cmdStartTime = time.Now()

	go func() {
		if err := docker.StartContainer(gui.client, ci.Container.ID); err != nil {
			gui.logError(fmt.Sprintf("Failed to start %s: %s", ci.Container.Name, err.Error()))
		} else {
			gui.logSuccess(fmt.Sprintf("Started %s", ci.Container.Name))
		}
		gui.running = false
	}()
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
	gui.running = true
	gui.runningCmd = "Reboot"
	gui.cmdStartTime = time.Now()

	go func() {
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
		gui.running = false
		gui.logSuccess(fmt.Sprintf("Reboot completed in %s", formatDuration(time.Since(gui.cmdStartTime))))
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

	gui.streamingLogs = true
	gui.streamingContainer = "kamal-proxy"
	gui.liveLogsStop = make(chan struct{})

	go func() {
		// Find kamal-proxy container
		cmd := `docker ps --filter "name=kamal-proxy" --format "{{.ID}}" | head -1`
		proxyID, err := gui.client.Run(cmd)
		if err != nil || strings.TrimSpace(proxyID) == "" {
			gui.logError("kamal-proxy container not found")
			gui.streamingLogs = false
			return
		}

		proxyID = strings.TrimSpace(proxyID)
		err = docker.StreamContainerLogs(gui.client, proxyID, func(line string) {
			gui.appendLog([]string{line})
			gui.g.Update(func(g *gocui.Gui) error { return nil })
		}, gui.liveLogsStop)

		gui.streamingLogs = false
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

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
