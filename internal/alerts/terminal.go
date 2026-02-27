// Package alerts provides terminal notification helpers.
package alerts

import (
	"fmt"
	"os"
)

// RingBell writes the ASCII BEL character to stderr, triggering a terminal
// bell or system notification in most terminal emulators.
func RingBell() {
	fmt.Fprint(os.Stderr, "\a")
}

// FlashAlert returns an ANSI escape sequence string that produces a brief
// reverse-video flash. The Bubble Tea model can emit this in its View output
// when it detects a new failure.
func FlashAlert() string {
	return "\033[?5h\033[?5l" // DECSCNM: reverse + normal
}
