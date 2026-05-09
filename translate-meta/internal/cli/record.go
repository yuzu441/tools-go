package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
	"go.opentelemetry.io/otel/trace"
)

func RunRecord(ctx context.Context, args []string) int {
	if len(args) != 1 {
		printRecordHelp()
		return ERROR
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.record")
	defer span.End()

	skillName := args[0]

	home, err := os.UserHomeDir()
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	claudeDirPath := filepath.Join(home, ".claude")
	jpSkillDirPath := filepath.Join(claudeDirPath, "skills-jp", skillName)

	hash, err := hashDir(ctx, jpSkillDirPath)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	status, _ := otelhelper.InSpan(ctx, "write metadata", func(ctx context.Context, _ trace.Span) (_ int, retErr error) {
		skillMetaPath := filepath.Join(claudeDirPath, "skills", skillName, metaFileName)
		if err := os.WriteFile(skillMetaPath, []byte(hash), 0644); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR, nil
		}

		return 0, nil
	})

	return status
}

func printRecordHelp() {
	fmt.Println(`Save current source state for a skill.

USAGE:
    translate-meta record <skill-name>

DESCRIPTION:
    Reads skills-jp/<skill-name>/ and writes translation metadata to
    skills/<skill-name>/.translate-meta.

NOTE:
    Run this after updating the translated files in skills/<skill-name>/.

EXAMPLE:
    translate-meta record coding-rules`)
}
