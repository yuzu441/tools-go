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

	if hasVersionFlag(args) {
		printVersion()
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
	case "manifest":
		return RunManifest(ctx, cfg, rest[1:])
	case "segments":
		return RunSegments(ctx, cfg, rest[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", rest[0])
		return 1
	}
}

func printRootHelp() {
	fmt.Println(`translate-meta

Track translation state of skills-jp -> skills, or of agents-jp -> agents.

USAGE:
    translate-meta [FLAGS] <COMMAND> <target-name>

FLAGS:
    --version               Print the translate-meta version and exit
    --skills-dir <DIR>      Directory containing translated skills
                            (default: <cwd>/skills)
    --skills-jp-dir <DIR>   Directory containing source skills
                            (default: <cwd>/skills-jp)
    --agents-dir <DIR>      Directory containing translated agents
                            (default: <cwd>/agents)
    --agents-jp-dir <DIR>   Directory containing source agents
                            (default: <cwd>/agents-jp)

    Agent mode (--agents-dir/--agents-jp-dir) and skill mode
    (--skills-dir/--skills-jp-dir) are mutually exclusive: combining a flag
    from one pair with a flag from the other is an error. In agent mode,
    <target-name> is one agent's name without the .md extension, mapping
    agents-jp/<target-name>.md to agents/<target-name>.md (a single file,
    not a directory).

COMMANDS:
    status      Check whether a skill/agent needs translation
    record      Save current source state after translation (writes JSON v2 meta)
    manifest    Output a JSON manifest of translation state and files
    segments    Segment-level translation-memory work-list and reassembly
    help        Show this help or the help of a command

EXAMPLES:
    translate-meta status coding-rules
    translate-meta --skills-dir ./skills --skills-jp-dir ./skills-jp record coding-rules
    translate-meta manifest coding-rules
    translate-meta segments status coding-rules
    translate-meta help segments
    translate-meta --agents-dir ./agents --agents-jp-dir ./agents-jp record reviewer
    translate-meta --agents-dir ./agents --agents-jp-dir ./agents-jp segments status reviewer`)
}
