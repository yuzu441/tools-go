package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
)

type manifestOutput struct {
	Skill string   `json:"skill"`
	State string   `json:"state"`
	Files []string `json:"files"`
}

func RunManifest(ctx context.Context, cfg Config, args []string) int {
	if len(args) != 1 {
		printManifestHelp()
		return ERROR
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.manifest")
	defer span.End()

	skillName := args[0]

	state, err := translationState(ctx, cfg, skillName)
	otelhelper.RecordError(span, err)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	files, err := listManifestFiles(filepath.Join(cfg.SkillsJPDir, skillName))
	if err != nil {
		otelhelper.RecordError(span, err)
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	out := manifestOutput{
		Skill: skillName,
		State: stateLabel(state),
		Files: files,
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	return 0
}

func listManifestFiles(root string) ([]string, error) {
	files := []string{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}

	slices.Sort(files)
	return files, nil
}

func printManifestHelp() {
	fmt.Println(`Output a JSON manifest describing a skill's translation state and files.

USAGE:
    translate-meta manifest <skill-name>

OUTPUT:
    A single JSON object on stdout:
        skill    Skill name
        state    up-to-date | needs-update | untranslated
        files    Sorted, slash-separated relative paths under skills-jp/<skill-name>/

EXIT CODES:
    0  success (regardless of which state was found)
    3  error

EXAMPLE:
    translate-meta manifest coding-rules`)
}
