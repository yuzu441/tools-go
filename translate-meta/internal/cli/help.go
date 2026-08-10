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
	case "manifest":
		printManifestHelp()
	case "segments":
		return runSegmentsHelp(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command for help: %s\n\n", args[0])
		printRootHelp()
		return 1
	}

	return 0
}

func runSegmentsHelp(args []string) int {
	if len(args) == 0 {
		printSegmentsHelp()
		return 0
	}

	switch args[0] {
	case "status":
		printSegmentsStatusHelp()
	case "assemble":
		printSegmentsAssembleHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown segments subcommand for help: %s\n\n", args[0])
		printSegmentsHelp()
		return 1
	}

	return 0
}
