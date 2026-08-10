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
	jpRoot := cfg.JPRoot(skillName)
	enRoot := cfg.ENRoot(skillName)
	metaPath := cfg.MetaPath(skillName)

	entries, err := cfg.ListFiles(skillName)
	if err != nil {
		otelhelper.RecordError(span, err)
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	files := make(map[string]fileMeta, len(entries))
	for _, e := range entries {
		fm, err := buildFileMeta(jpRoot, enRoot, e)
		if err != nil {
			otelhelper.RecordError(span, err)
			slog.ErrorContext(ctx, err.Error())
			return ERROR
		}
		files[e.Path] = fm
	}

	status, _ := otelhelper.InSpan(ctx, "write metadata", func(ctx context.Context, _ trace.Span) (_ int, retErr error) {
		out, err := marshalMetaV2(metaV2{Version: metaVersion, Files: files})
		if err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR, nil
		}
		if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR, nil
		}
		if err := os.WriteFile(metaPath, out, 0o644); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR, nil
		}

		return 0, nil
	})

	return status
}

func buildFileMeta(jpRoot, enRoot string, e fileEntry) (fileMeta, error) {
	switch e.Policy {
	case "preserve":
		return fileMeta{Policy: "preserve"}, nil
	case "copy":
		content, err := os.ReadFile(filepath.Join(jpRoot, filepath.FromSlash(e.Path)))
		if err != nil {
			return fileMeta{}, fmt.Errorf("read %s: %w", e.Path, err)
		}
		return fileMeta{Policy: "copy", Hash: hashSegment(string(content))}, nil
	case "translate":
		return buildTranslateFileMeta(jpRoot, enRoot, e.Path)
	default:
		return fileMeta{}, fmt.Errorf("unknown policy %q for %s", e.Policy, e.Path)
	}
}

// buildTranslateFileMeta segments both the Japanese source and its current
// English translation (if any) and pairs them positionally by index. This
// doubles as translation-memory bootstrap for pre-existing jp/en pairs: no
// separate migration command is needed. When the segment counts don't
// match (or no translated file exists yet), the entry degrades to a
// whole-file hash and a warning is printed, so the next record recovers
// automatically once the mismatch is resolved.
func buildTranslateFileMeta(jpRoot, enRoot, relPath string) (fileMeta, error) {
	jpContent, err := os.ReadFile(filepath.Join(jpRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return fileMeta{}, fmt.Errorf("read %s: %w", relPath, err)
	}

	jpBlocks, err := splitSegments(string(jpContent))
	if err != nil {
		return fileMeta{}, fmt.Errorf("segment %s (jp source): %w", relPath, err)
	}
	jpSegs := translatableSegments(jpBlocks)

	enSegs, ok := readEnglishSegments(enRoot, relPath)
	if !ok || len(enSegs) != len(jpSegs) {
		return degradeToWholeFile(relPath, string(jpContent), enSegs, jpSegs), nil
	}

	segments := make([]segmentPair, len(jpSegs))
	for i := range jpSegs {
		segments[i] = segmentPair{JPHash: hashSegment(jpSegs[i]), ENHash: hashSegment(enSegs[i])}
	}
	return fileMeta{Policy: "translate", Segments: segments}, nil
}

// readEnglishSegments returns the translatable segments of the current
// English file at enRoot/relPath. ok is false if the file does not exist
// or cannot be segmented (its content is treated as absent, not an error,
// so record can still produce a degraded entry for the caller).
func readEnglishSegments(enRoot, relPath string) (segs []string, ok bool) {
	enContent, err := os.ReadFile(filepath.Join(enRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, false
	}
	enBlocks, err := splitSegments(string(enContent))
	if err != nil {
		return nil, false
	}
	return translatableSegments(enBlocks), true
}

func degradeToWholeFile(relPath, jpContent string, enSegs, jpSegs []string) fileMeta {
	fmt.Fprintf(os.Stderr, "warning: %s: could not pair Japanese and English segments (jp=%d, en=%d); degrading to whole-file tracking\n", relPath, len(jpSegs), len(enSegs))
	return fileMeta{Policy: "translate", Hash: hashSegment(jpContent)}
}

func printRecordHelp() {
	fmt.Println(`Save current source state for a skill (or, with
--agents-dir/--agents-jp-dir, an agent).

USAGE:
    translate-meta record <skill-name>

DESCRIPTION:
    Reads skills-jp/<skill-name>/ and writes translation metadata to
    skills/<skill-name>/.translate-meta as JSON (v2 format). Files with
    policy "translate" are segmented and paired positionally with the
    current English translation at skills/<skill-name>/, which both
    records per-segment hashes and bootstraps translation memory for
    pre-existing jp/en pairs. A legacy (pre-v2) meta file is upgraded to v2
    automatically the next time record runs.

    With --agents-dir/--agents-jp-dir, <skill-name> is an agent name
    (without .md), mapping agents-jp/<skill-name>.md to
    agents/<skill-name>.md. Metadata is written to
    agents/.translate-meta/<skill-name>.json instead, since agents share a
    single flat directory rather than one directory per target.

NOTE:
    Run this after updating the translated files in skills/<skill-name>/
    (or agents/<skill-name>.md in agent mode).

EXAMPLE:
    translate-meta record coding-rules
    translate-meta --agents-dir ./agents --agents-jp-dir ./agents-jp record reviewer`)
}
