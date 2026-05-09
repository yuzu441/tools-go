package cli

import (
	"fmt"
	"os"
)

func RunHelp(args []string) int {
	if len(args) == 0 {
		printRootHelp()
		return 0
	}

	switch args[0] {
	case "status":
		printStautsHelp()
	case "record":
		printRecordHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command for help: %s\n\n", args[0])
		printRootHelp()
		return 1
	}

	return 0
}
