package gui

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"milliseconds", 500 * time.Millisecond, "500ms"},
		{"seconds", 5 * time.Second, "5.0s"},
		{"seconds with ms", 5500 * time.Millisecond, "5.5s"},
		{"minutes", 2*time.Minute + 30*time.Second, "2m30s"},
		{"hours", 1*time.Hour + 15*time.Minute, "1h15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"hello", 5, "hello"},
		{"hello", 4, "h..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 10, "hello     "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := padRight(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
	}
}

func TestPadLeft(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 10, "     hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := padLeft(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("padLeft(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
	}
}

func TestCenter(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hi", 6, "  hi  "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"},
		{"a", 4, " a  "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := center(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("center(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
	}
}

func TestProgressBar(t *testing.T) {
	tests := []struct {
		percent  int
		width    int
		expected string
	}{
		{0, 10, "[░░░░░░░░░░] 0%"},
		{50, 10, "[█████░░░░░] 50%"},
		{100, 10, "[██████████] 100%"},
		{-10, 10, "[░░░░░░░░░░] 0%"},
		{150, 10, "[██████████] 100%"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := progressBar(tt.percent, tt.width)
			if result != tt.expected {
				t.Errorf("progressBar(%d, %d) = %q, want %q", tt.percent, tt.width, result, tt.expected)
			}
		})
	}
}

func TestSeparator(t *testing.T) {
	result := separator(5, "-")
	if result != "-----" {
		t.Errorf("separator(5, \"-\") = %q, want \"-----\"", result)
	}
}
