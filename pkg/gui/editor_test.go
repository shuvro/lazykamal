package gui

import (
	"testing"
	"unicode/utf8"
)

func TestRuneIndexToByteOffset(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		runeIdx  int
		expected int
	}{
		{"empty string", "", 0, 0},
		{"ascii at 0", "hello", 0, 0},
		{"ascii at 3", "hello", 3, 3},
		{"ascii at end", "hello", 5, 5},
		{"multibyte rune 0", "héllo", 0, 0},
		{"multibyte rune 1", "héllo", 1, 1},
		{"multibyte after é", "héllo", 2, 3},
		{"multibyte at end", "héllo", 5, 6},
		{"cjk rune 0", "日本語", 0, 0},
		{"cjk rune 1", "日本語", 1, 3},
		{"cjk rune 2", "日本語", 2, 6},
		{"cjk rune 3 (end)", "日本語", 3, 9},
		{"emoji", "a😀b", 1, 1},
		{"after emoji", "a😀b", 2, 5},
		{"end after emoji", "a😀b", 3, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runeIndexToByteOffset(tt.s, tt.runeIdx)
			if got != tt.expected {
				t.Errorf("runeIndexToByteOffset(%q, %d) = %d, want %d", tt.s, tt.runeIdx, got, tt.expected)
			}
		})
	}
}

func TestEditorUTF8Insert(t *testing.T) {
	// Simulate inserting a multi-byte character
	line := "hello"
	col := 2 // rune index 2 = after "he"

	byteOff := runeIndexToByteOffset(line, col)
	left := line[:byteOff]
	right := line[byteOff:]
	newLine := left + "é" + right
	col++ // rune index increments by 1

	if newLine != "heéllo" {
		t.Errorf("Insert result = %q, want %q", newLine, "heéllo")
	}
	if col != 3 {
		t.Errorf("Col after insert = %d, want 3", col)
	}

	// Verify we can insert again at the new position
	byteOff = runeIndexToByteOffset(newLine, col)
	left = newLine[:byteOff]
	right = newLine[byteOff:]
	newLine = left + "ñ" + right
	col++

	if newLine != "heéñllo" {
		t.Errorf("Second insert result = %q, want %q", newLine, "heéñllo")
	}
	if col != 4 {
		t.Errorf("Col after second insert = %d, want 4", col)
	}
}

func TestEditorUTF8Backspace(t *testing.T) {
	// Simulate backspace on a multi-byte character
	line := "héllo"
	col := 2 // rune index 2 = after "hé"

	byteOffCur := runeIndexToByteOffset(line, col)
	byteOffPrev := runeIndexToByteOffset(line, col-1)
	newLine := line[:byteOffPrev] + line[byteOffCur:]
	col--

	if newLine != "hllo" {
		t.Errorf("Backspace result = %q, want %q", newLine, "hllo")
	}
	if col != 1 {
		t.Errorf("Col after backspace = %d, want 1", col)
	}
}

func TestEditorUTF8MoveRight(t *testing.T) {
	line := "日本語"
	runeCount := utf8.RuneCountInString(line)
	if runeCount != 3 {
		t.Fatalf("Expected 3 runes, got %d", runeCount)
	}

	// Moving right from col 0 should go to 1, not 3 (byte offset)
	col := 0
	if col < runeCount {
		col++
	}
	if col != 1 {
		t.Errorf("Col after move right = %d, want 1", col)
	}

	// At the end, should not move further
	col = runeCount
	if col < runeCount {
		col++
	}
	if col != 3 {
		t.Errorf("Col at end = %d, want 3", col)
	}
}

func TestEditorUTF8Enter(t *testing.T) {
	// Split a line with multi-byte chars
	line := "héllo wörld"
	col := 6 // rune index 6 = after "héllo "

	byteOff := runeIndexToByteOffset(line, col)
	left := line[:byteOff]
	right := line[byteOff:]

	if left != "héllo " {
		t.Errorf("Left after enter = %q, want %q", left, "héllo ")
	}
	if right != "wörld" {
		t.Errorf("Right after enter = %q, want %q", right, "wörld")
	}
}
