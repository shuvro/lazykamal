package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jroimartin/gocui"
)

const viewEditor = "editor"
const viewEditorStatus = "editorStatus"

// Editor state: nano/vi-style in-TUI modal.
type editorState struct {
	Path        string
	Lines       []string
	Row         int
	Col         int
	Scroll      int
	Dirty       bool
	PrevScreen  Screen
	ConfirmQuit bool // show "Quit without saving? (y/n)"
}

func (gui *GUI) openEditor(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		gui.appendLog([]string{"Could not read file: " + err.Error()})
		return false
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{""}
	}
	gui.editor = &editorState{
		Path:       path,
		Lines:      lines,
		Row:        0,
		Col:        0,
		Scroll:     0,
		Dirty:      false,
		PrevScreen: gui.screen,
	}
	gui.screen = ScreenEditor
	return true
}

func (gui *GUI) closeEditor() {
	if gui.editor != nil {
		path := gui.editor.Path
		gui.screen = gui.editor.PrevScreen
		gui.editor = nil
		gui.g.SetCurrentView(viewMain)
		if strings.Contains(filepath.ToSlash(path), "config/") {
			gui.refreshDestinations()
		}
	} else {
		gui.g.SetCurrentView(viewMain)
	}
}

func (gui *GUI) editorSave() bool {
	if gui.editor == nil {
		return false
	}
	data := []byte(strings.Join(gui.editor.Lines, "\n"))
	// Use 0600 for secrets files for better security
	perm := os.FileMode(0644)
	if strings.Contains(gui.editor.Path, "secrets") {
		perm = 0600
	}
	if err := os.WriteFile(gui.editor.Path, data, perm); err != nil {
		gui.appendLog([]string{"Could not save: " + err.Error()})
		return false
	}
	gui.editor.Dirty = false
	return true
}

func (gui *GUI) editorQuit() {
	if gui.editor == nil {
		return
	}
	if gui.editor.ConfirmQuit {
		gui.editor.ConfirmQuit = false
		gui.closeEditor()
		return
	}
	if gui.editor.Dirty {
		gui.editor.ConfirmQuit = true
		return
	}
	gui.closeEditor()
}

func (gui *GUI) editorConfirmQuitYes() {
	if gui.editor != nil && gui.editor.ConfirmQuit {
		gui.editor.ConfirmQuit = false
		gui.closeEditor()
	}
}

func (gui *GUI) editorConfirmQuitNo() {
	if gui.editor != nil && gui.editor.ConfirmQuit {
		gui.editor.ConfirmQuit = false
	}
}

func (gui *GUI) editorMoveUp() {
	if gui.editor == nil {
		return
	}
	if gui.editor.Row > 0 {
		gui.editor.Row--
		if gui.editor.Col > len(gui.editor.Lines[gui.editor.Row]) {
			gui.editor.Col = len(gui.editor.Lines[gui.editor.Row])
		}
		if gui.editor.Row < gui.editor.Scroll {
			gui.editor.Scroll = gui.editor.Row
		}
	}
}

func (gui *GUI) editorMoveDown() {
	if gui.editor == nil {
		return
	}
	if gui.editor.Row < len(gui.editor.Lines)-1 {
		gui.editor.Row++
		if gui.editor.Col > len(gui.editor.Lines[gui.editor.Row]) {
			gui.editor.Col = len(gui.editor.Lines[gui.editor.Row])
		}
	}
}

func (gui *GUI) editorMoveLeft() {
	if gui.editor == nil {
		return
	}
	if gui.editor.Col > 0 {
		gui.editor.Col--
	}
}

func (gui *GUI) editorMoveRight() {
	if gui.editor == nil {
		return
	}
	lineLen := len(gui.editor.Lines[gui.editor.Row])
	if gui.editor.Col < lineLen {
		gui.editor.Col++
	}
}

func (gui *GUI) editorInsertRune(r rune) {
	if gui.editor == nil {
		return
	}
	line := gui.editor.Lines[gui.editor.Row]
	left := line[:gui.editor.Col]
	right := line[gui.editor.Col:]
	gui.editor.Lines[gui.editor.Row] = left + string(r) + right
	gui.editor.Col += utf8.RuneLen(r)
	gui.editor.Dirty = true
}

func (gui *GUI) editorBackspace() {
	if gui.editor == nil {
		return
	}
	if gui.editor.Col > 0 {
		line := gui.editor.Lines[gui.editor.Row]
		gui.editor.Lines[gui.editor.Row] = line[:gui.editor.Col-1] + line[gui.editor.Col:]
		gui.editor.Col--
		gui.editor.Dirty = true
	} else if gui.editor.Row > 0 {
		// Merge with previous line
		prevLen := len(gui.editor.Lines[gui.editor.Row-1])
		gui.editor.Lines[gui.editor.Row-1] += gui.editor.Lines[gui.editor.Row]
		gui.editor.Lines = append(gui.editor.Lines[:gui.editor.Row], gui.editor.Lines[gui.editor.Row+1:]...)
		gui.editor.Row--
		gui.editor.Col = prevLen
		if gui.editor.Scroll > 0 {
			gui.editor.Scroll--
		}
		gui.editor.Dirty = true
	}
}

func (gui *GUI) editorEnter() {
	if gui.editor == nil {
		return
	}
	line := gui.editor.Lines[gui.editor.Row]
	left := line[:gui.editor.Col]
	right := line[gui.editor.Col:]
	gui.editor.Lines[gui.editor.Row] = left
	newLine := right
	gui.editor.Lines = append(gui.editor.Lines[:gui.editor.Row+1], append([]string{newLine}, gui.editor.Lines[gui.editor.Row+1:]...)...)
	gui.editor.Row++
	gui.editor.Col = 0
	gui.editor.Dirty = true
}

func (gui *GUI) renderEditorView(g *gocui.Gui) error {
	if gui.editor == nil {
		return nil
	}
	maxX, maxY := g.Size()
	if maxY < 8 {
		maxY = 8
	}
	// Editor area: full screen minus one line for status
	editorH := maxY - 1
	if v, err := g.SetView(viewEditor, 0, 0, maxX-1, editorH-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = true
		v.Title = " Edit file (nano/vi style) "
		v.Wrap = false
		v.Editable = false
	}
	v, _ := g.View(viewEditor)
	if v == nil {
		return nil
	}
	v.Clear()
	// Scroll: ensure cursor is visible
	_, vy := v.Size()
	if gui.editor.Row >= gui.editor.Scroll+vy {
		gui.editor.Scroll = gui.editor.Row - vy + 1
	}
	if gui.editor.Row < gui.editor.Scroll {
		gui.editor.Scroll = gui.editor.Row
	}
	start := gui.editor.Scroll
	end := start + vy
	if end > len(gui.editor.Lines) {
		end = len(gui.editor.Lines)
	}
	for i := start; i < end; i++ {
		line := gui.editor.Lines[i]
		if i == gui.editor.Row {
			fmt.Fprintf(v, "%s\n", line)
		} else {
			fmt.Fprintln(v, line)
		}
	}
	v.SetCursor(gui.editor.Col, gui.editor.Row-gui.editor.Scroll)
	g.SetCurrentView(viewEditor)

	// Status line at bottom
	if _, err := g.SetView(viewEditorStatus, 0, editorH, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	s, _ := g.View(viewEditorStatus)
	if s != nil {
		s.Clear()
		status := gui.editor.Path
		if gui.editor.Dirty {
			status += " [Modified]"
		}
		if gui.editor.ConfirmQuit {
			status = " Quit without saving? (y/n) "
		} else {
			status += "  ^S Save  ^Q Esc Quit  Arrows move"
		}
		fmt.Fprint(s, status)
	}
	return nil
}
