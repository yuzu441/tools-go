package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
)

const (
	UP_TO_DATE = iota
	NEEDS_UPDATE
	UNTRANSLATED
	ERROR
)

const metaFileName = ".translate-meta"

func RunStatus(ctx context.Context, args []string) int {
	if len(args) != 1 {
		printStautsHelp()
		return ERROR
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.status")
	defer span.End()

	skillName := args[0]

	home, err := os.UserHomeDir()
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	claudeDir := filepath.Join(home, ".claude")

	skillsMetaPath := filepath.Join(claudeDir, "skills", skillName, metaFileName)
	if _, err := os.Stat(skillsMetaPath); os.IsNotExist(err) {
		return UNTRANSLATED
	}
	transHash, err := readMetaFile(skillsMetaPath)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}
	// metaデータがないので強制的に作る
	if transHash == "" {
		return NEEDS_UPDATE
	}

	skillsJPPath := filepath.Join(claudeDir, "skills-jp", skillName)
	hash, err := hashDir(ctx, skillsJPPath)
	otelhelper.RecordError(span, err)
	if err != nil {
		slog.Error(err.Error())
		return ERROR
	}

	if hash != transHash {
		return NEEDS_UPDATE
	}
	return UP_TO_DATE
}

func hashDir(ctx context.Context, root string) (_ string, retErr error) {
	ctx, span := otelhelper.StartSpan(ctx, "hashDir")
	defer func() {
		otelhelper.RecordError(span, retErr)
		span.End()
	}()

	relPaths, err := otelhelper.InSpan(ctx, "hashDir.walkDir", func(ctx context.Context, _ trace.Span) ([]string, error) {
		var paths []string

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

			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("walk dir: %w", err)
		}

		return paths, nil
	})

	if err != nil {
		return "", err
	}

	slices.Sort(relPaths)
	span.SetAttributes(attribute.Int("file.count", len(relPaths)))

	h := sha256.New()

	return otelhelper.InSpan(ctx, "hashDir.hashFiles", func(ctx context.Context, _ trace.Span) (string, error) {
		for _, rel := range relPaths {
			full := filepath.Join(root, filepath.FromSlash(rel))

			if _, err := io.WriteString(h, "path:"+rel+"\n"); err != nil {
				return "", fmt.Errorf("write path to hash: %w", err)
			}

			f, err := os.Open(full)
			if err != nil {
				return "", fmt.Errorf("open file %s: %w", rel, err)
			}

			if _, err := io.Copy(h, f); err != nil {
				f.Close()
				return "", fmt.Errorf("hash file %s: %w", rel, err)
			}

			if err := f.Close(); err != nil {
				return "", fmt.Errorf("close file %s: %w", rel, err)
			}

			if _, err := io.WriteString(h, "\n"); err != nil {
				return "", fmt.Errorf("write delimiter: %w", err)
			}
		}

		return hex.EncodeToString(h.Sum(nil)), nil
	})
}

func readMetaFile(path string) (string, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		// TODO: 外部でNotExsistsチェックをしてもらう想定でerrorをラップする。またerrorライブラリも調査
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// TODO: 本来は`key=value`形式の想定なのでパース処理を追加する
	return string(content), nil
}

func printStautsHelp() {
	fmt.Println(`Check whether a skill is up-to-date with its source.

USAGE:
    translate-meta status <skill-name>

OUTPUT:
    up-to-date      Source matches recorded state
    needs-update    Source differs from recorded state
    untranslated    No translation record exists

EXIT CODES:
    0  up-to-date
    1  needs-update
    2  untranslated
    3  error

EXAMPLE:
    translate-meta status coding-rules`)
}
