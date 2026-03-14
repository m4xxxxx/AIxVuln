package misc

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

// PanicWithStack prints a timestamped fatal message with stack trace, then panics.
// It avoids log.Fatal/Fatalln/Fatalf so the failure reason is easier to diagnose.
func PanicWithStack(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006-01-02 15:04:05")
	_, _ = fmt.Fprintf(os.Stderr, "[%s][FATAL] %s\n%s", ts, msg, debug.Stack())
	panic(msg)
}
