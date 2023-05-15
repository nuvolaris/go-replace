package replace

import (
	"fmt"
	"os"
)

// Log message
func logMessage(message string) {
	if opts.Verbose {
		fmt.Fprint(os.Stderr, message)
	}
}

// Log error object as message
func logError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
}
