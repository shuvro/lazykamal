package gui

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

const viewConfirm = "confirm"

// ConfirmDialog state
type confirmState struct {
	Title    string
	Message  string
	OnYes    func()
	OnNo     func()
	Selected int // 0 = Yes, 1 = No
}

func (gui *GUI) showConfirm(title, message string, onYes, onNo func()) {
	gui.confirm = &confirmState{
		Title:    title,
		Message:  message,
		OnYes:    onYes,
		OnNo:     onNo,
		Selected: 1, // Default to "No" for safety
	}
	gui.screen = ScreenConfirm
}

func (gui *GUI) renderConfirmDialog(g *gocui.Gui) error {
	if gui.confirm == nil {
		return nil
	}

	maxX, maxY := g.Size()

	// Dialog dimensions
	width := 50
	height := 7
	if width > maxX-4 {
		width = maxX - 4
	}

	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2
	x1 := x0 + width
	y1 := y0 + height

	if v, err := g.SetView(viewConfirm, x0, y0, x1, y1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " " + gui.confirm.Title + " "
		v.FgColor = gocui.ColorWhite
	}

	v, _ := g.View(viewConfirm)
	if v == nil {
		return nil
	}
	v.Clear()

	// Message
	fmt.Fprintln(v)
	fmt.Fprintf(v, " %s\n", gui.confirm.Message)
	fmt.Fprintln(v)

	// Buttons
	yesStyle := "  [ Yes ]  "
	noStyle := "  [ No ]  "

	if gui.confirm.Selected == 0 {
		yesStyle = " " + cyan(iconArrow) + green("[ Yes ]") + "  "
	} else {
		noStyle = " " + cyan(iconArrow) + red("[ No ]") + "  "
	}

	fmt.Fprintf(v, "       %s    %s\n", yesStyle, noStyle)

	g.SetCurrentView(viewConfirm)
	return nil
}

func (gui *GUI) confirmLeft() {
	if gui.confirm != nil && gui.confirm.Selected > 0 {
		gui.confirm.Selected--
	}
}

func (gui *GUI) confirmRight() {
	if gui.confirm != nil && gui.confirm.Selected < 1 {
		gui.confirm.Selected++
	}
}

func (gui *GUI) confirmEnter() {
	if gui.confirm == nil {
		return
	}

	if gui.confirm.Selected == 0 && gui.confirm.OnYes != nil {
		gui.confirm.OnYes()
	} else if gui.confirm.Selected == 1 && gui.confirm.OnNo != nil {
		gui.confirm.OnNo()
	}

	gui.closeConfirm()
}

func (gui *GUI) closeConfirm() {
	gui.g.DeleteView(viewConfirm)
	gui.confirm = nil
	gui.screen = gui.prevScreen
	gui.g.SetCurrentView(viewMain)
}

// getDestructiveMessage returns a warning message for destructive actions
func getDestructiveMessage(screen Screen, idx int) string {
	switch screen {
	case ScreenDeploy:
		if idx == 3 {
			return "Rollback to previous version?"
		}
	case ScreenApp:
		if idx == 2 {
			return "Stop the application?"
		}
		if idx == 13 {
			return "Remove the application? This cannot be undone."
		}
		if idx == 15 {
			return "Stop and remove stale containers?"
		}
	case ScreenAccessory:
		if idx == 2 {
			return "Stop all accessories?"
		}
		if idx == 5 {
			return "Remove all accessories? This cannot be undone."
		}
	case ScreenProxy:
		if idx == 2 {
			return "Stop the proxy?"
		}
		if idx == 8 {
			return "Remove the proxy? This cannot be undone."
		}
	case ScreenOther:
		if idx == 8 {
			return "Force release the lock?"
		}
		if idx == 13 {
			return "Delete environment variables?"
		}
	case ScreenBuild:
		if idx == 5 {
			return "Remove the build setup?"
		}
	case ScreenPrune:
		if idx == 0 {
			return "Prune all old images and containers?"
		}
		if idx == 1 {
			return "Prune old images?"
		}
		if idx == 2 {
			return "Prune old containers?"
		}
	case ScreenRegistry:
		if idx == 3 {
			return "Remove registry configuration?"
		}
	}
	return "Are you sure you want to proceed?"
}
