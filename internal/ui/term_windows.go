//go:build windows

package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT turns on ANSI escape sequence processing for the console behind
// f. Windows Terminal has it on already; the classic console (conhost) does
// not, and without it colors and cursor movement print as raw "\x1b[..."
// garbage. It reports false where it cannot be enabled, so the printer falls
// back to plain text.
func enableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
