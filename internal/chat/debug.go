package chat

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
)

const (
	ansiReset = "\033[0m"
	ansiCyan  = "\033[36m"
	ansiGreen = "\033[32m"
)

func debugLog(label, value string) {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Printf("%s%s:%s %s\n", ansiCyan, label, ansiGreen, value)
		return
	}
	fmt.Printf("%s: %s\n", label, value)
}
