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

func RunRecord(ctx context.Context, cfg Config, args []string) int {
	if len(args) != 1 {
		printRecordHelp()
		return ERROR
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.record")
	defer span.End()

	skillName := args[0]
	jpSkillDirPath := filepath.Join(cfg.SkillsJPDir, skillName)

	hash, err := hashDir(ctx, jpSkillDirPath)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	status, _ := otelhelper.InSpan(ctx, "write metadata", func(ctx context.Context, _ trace.Span) (_ int, retErr error) {
		skillMetaDir := filepath.Join(cfg.SkillsDir, skillName)
		if err := os.MkdirAll(skillMetaDir, 0o755); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR, nil
		}
		skillMetaPath := filepath.Join(skillMetaDir, metaFileName)
		if err := os.WriteFile(skillMetaPath, []byte(hash), 0o644); err != nil {
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
