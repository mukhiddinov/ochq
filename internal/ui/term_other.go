//go:build !windows

package ui

import "os"

// enableVT reports whether f understands ANSI escape sequences; every
// terminal outside Windows does.
func enableVT(*os.File) bool { return true }
