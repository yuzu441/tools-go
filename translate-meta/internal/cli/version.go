package cli

import (
	"fmt"
	"slices"
)

// version is maintained here rather than derived from build metadata: this
// monorepo holds several tools, so a single repository-wide git tag cannot
// version them independently, and installs reference @main. Bump this by hand
// when releasing translate-meta.
const version = "v2.0.0"

func hasVersionFlag(args []string) bool {
	return slices.Contains(args, "--version") || slices.Contains(args, "-version")
}

func printVersion() {
	fmt.Printf("translate-meta %s\n", version)
}
