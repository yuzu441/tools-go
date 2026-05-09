package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/attribute"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
)

func Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printRootHelp()
		return 0
	}

	cfg, rest, err := Resolve(args)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return 1
	}

	if len(rest) == 0 {
		printRootHelp()
		return 0
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.run")
	defer span.End()
	span.SetAttributes(attribute.String("cli.command", rest[0]))

	switch rest[0] {
	case "help":
		return RunHelp(rest[1:])
	case "status":
		return RunStatus(ctx, cfg, rest[1:])
	case "record":
		return RunRecord(ctx, cfg, rest[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", rest[0])
		return 1
	}
}

func printRootHelp() {
	fmt.Println(`translate-meta

Track translation state of skills-jp -> skills.

USAGE:
    translate-meta [FLAGS] <COMMAND> <skill-name>

FLAGS:
    --skills-dir <DIR>      Directory containing translated skills
                            (default: <cwd>/skills)
    --skills-jp-dir <DIR>   Directory containing source skills
                            (default: <cwd>/skills-jp)

COMMANDS:
    status    Check whether a skill needs translation
    record    Save current source state after translation
    help      Show this help or the help of a command

EXAMPLES:
    translate-meta status coding-rules
    translate-meta --skills-dir ./skills --skills-jp-dir ./skills-jp record coding-rules
    translate-meta help status`)
}
