package cli

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
)

func Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printRootHelp()
		return 0
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.run")
	defer span.End()
	span.SetAttributes(attribute.String("cli.command", args[0]))

	switch args[0] {
	case "help":
		return RunHelp(args[1:])
	case "status":
		return RunStatus(ctx, args[1:])
	case "record":
		return RunRecord(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command for help: %s\n\n", args[0])
		return 1
	}
}

func printRootHelp() {
	fmt.Println(`translate-meta

Track translation state of skills-jp -> skills.

USAGE:
    translate-meta <COMMAND> <skill-name>

COMMANDS:
    status    Check whether a skill needs translation
    record    Save current source state after translation
    help      Show this help or the help of a command

EXAMPLES:
    translate-meta status coding-rules
    translate-meta record coding-rules
    translate-meta help status`)
}
