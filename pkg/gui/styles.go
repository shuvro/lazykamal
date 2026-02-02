package gui

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color codes for terminal styling
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorBgRed   = "\033[41m"
	colorBgGreen = "\033[42m"
)

// Spinner frames for loading animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Status icons
const (
	iconSuccess  = "✓"
	iconError    = "✗"
	iconRunning  = "●"
	iconPending  = "○"
	iconWarning  = "⚠"
	iconInfo     = "ℹ"
	iconArrow    = "›"
	iconDot      = "•"
	iconCheck    = "✔"
	iconCross    = "✘"
	iconStar     = "★"
	iconPlay     = "▶"
	iconStop     = "■"
	iconPause    = "❚❚"
	iconRefresh  = "↻"
	iconFolder   = "📁"
	iconFile     = "📄"
	iconGear     = "⚙"
	iconRocket   = "🚀"
	iconServer   = "🖥"
	iconLock     = "🔒"
	iconUnlock   = "🔓"
	iconKey      = "🔑"
	iconPackage  = "📦"
	iconTerminal = "⌨"
)

// Styled text helpers
func colorize(text, color string) string {
	return color + text + colorReset
}

func green(text string) string  { return colorize(text, colorGreen) }
func red(text string) string    { return colorize(text, colorRed) }
func yellow(text string) string { return colorize(text, colorYellow) }
func blue(text string) string   { return colorize(text, colorBlue) }
func cyan(text string) string   { return colorize(text, colorCyan) }
func dim(text string) string    { return colorize(text, colorDim) }
func bold(text string) string   { return colorize(text, colorBold) }

// StatusLine creates a colored status line
func statusLine(status, message string) string {
	switch status {
	case "success":
		return green(iconSuccess) + " " + message
	case "error":
		return red(iconError) + " " + message
	case "running":
		return yellow(iconRunning) + " " + message
	case "warning":
		return yellow(iconWarning) + " " + message
	case "info":
		return blue(iconInfo) + " " + message
	default:
		return message
	}
}

// FormatDuration formats a duration in human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

// FormatTimestamp formats a timestamp for display
func formatTimestamp(t time.Time) string {
	return t.Format("15:04:05")
}

// TimestampedLine creates a log line with timestamp
func timestampedLine(line string) string {
	return dim(formatTimestamp(time.Now())) + " " + line
}

// ProgressBar creates a simple progress bar
func progressBar(percent, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := width * percent / 100
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("[%s] %d%%", bar, percent)
}

// Separator creates a horizontal separator line
func separator(width int, char string) string {
	return strings.Repeat(char, width)
}

// Truncate truncates a string to max length with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// PadRight pads a string to the right with spaces
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// PadLeft pads a string to the left with spaces
func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// Center centers a string within a width
func center(s string, width int) string {
	if len(s) >= width {
		return s
	}
	padding := (width - len(s)) / 2
	return strings.Repeat(" ", padding) + s + strings.Repeat(" ", width-len(s)-padding)
}
