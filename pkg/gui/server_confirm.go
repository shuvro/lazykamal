package gui

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

const viewServerConfirm = "serverConfirm"

func (gui *ServerGUI) showConfirm(title, message string, onYes, onNo func()) {
	gui.confirm = &confirmState{
		Title:    title,
		Message:  message,
		OnYes:    onYes,
		OnNo:     onNo,
		Selected: 1, // Default to "No" for safety
	}
	gui.prevScreen = gui.screen
	gui.screen = ServerScreenConfirm
}

func (gui *ServerGUI) renderConfirmDialog(g *gocui.Gui) error {
	if gui.confirm == nil {
		return nil
	}

	maxX, maxY := g.Size()

	width := 50
	height := 7
	if width > maxX-4 {
		width = maxX - 4
	}

	x0 := (maxX - width) / 2
	y0 := (maxY - height) / 2
	x1 := x0 + width
	y1 := y0 + height

	if v, err := g.SetView(viewServerConfirm, x0, y0, x1, y1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " " + gui.confirm.Title + " "
		v.FgColor = gocui.ColorWhite
	}

	v, _ := g.View(viewServerConfirm)
	if v == nil {
		return nil
	}
	v.Clear()

	fmt.Fprintln(v)
	fmt.Fprintf(v, " %s\n", gui.confirm.Message)
	fmt.Fprintln(v)

	yesStyle := "  [ Yes ]  "
	noStyle := "  [ No ]  "

	if gui.confirm.Selected == 0 {
		yesStyle = " " + cyan(iconArrow) + green("[ Yes ]") + "  "
	} else {
		noStyle = " " + cyan(iconArrow) + red("[ No ]") + "  "
	}

	fmt.Fprintf(v, "       %s    %s\n", yesStyle, noStyle)

	g.SetCurrentView(viewServerConfirm)
	return nil
}

func (gui *ServerGUI) confirmLeft() {
	if gui.confirm != nil && gui.confirm.Selected > 0 {
		gui.confirm.Selected--
	}
}

func (gui *ServerGUI) confirmRight() {
	if gui.confirm != nil && gui.confirm.Selected < 1 {
		gui.confirm.Selected++
	}
}

func (gui *ServerGUI) confirmEnter() {
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

func (gui *ServerGUI) closeConfirm() {
	gui.g.DeleteView(viewServerConfirm)
	gui.confirm = nil
	gui.screen = gui.prevScreen
	gui.g.SetCurrentView(viewMain)
}
