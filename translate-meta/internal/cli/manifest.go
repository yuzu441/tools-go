package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
)

type fileEntry struct {
	Path   string `json:"path"`
	Policy string `json:"policy"`
	Reason string `json:"reason,omitempty"`
}

type manifestOutput struct {
	Skill   string      `json:"skill"`
	State   string      `json:"state"`
	Files   []fileEntry `json:"files"`
	Orphans []string    `json:"orphans"`
}

var codeExtensions = map[string]bool{
	".js":     true,
	".mjs":    true,
	".cjs":    true,
	".ts":     true,
	".tsx":    true,
	".jsx":    true,
	".css":    true,
	".scss":   true,
	".sass":   true,
	".less":   true,
	".py":     true,
	".go":     true,
	".sh":     true,
	".rb":     true,
	".rs":     true,
	".java":   true,
	".kt":     true,
	".swift":  true,
	".c":      true,
	".cpp":    true,
	".h":      true,
	".html":   true,
	".vue":    true,
	".svelte": true,
	".yaml":   true,
	".yml":    true,
	".toml":   true,
}

var (
	frontmatterRE = regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)
	fencedCodeRE  = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRE  = regexp.MustCompile("`[^`\n]*`")
)

const excludeDoNotEdit = "DO_NOT_EDIT.txt"

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

	orphans, err := listOrphans(cfg.SkillsDir, cfg.SkillsJPDir, skillName)
	if err != nil {
		otelhelper.RecordError(span, err)
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	out := manifestOutput{
		Skill:   skillName,
		State:   stateLabel(state),
		Files:   files,
		Orphans: orphans,
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	return 0
}

func listManifestFiles(root string) ([]fileEntry, error) {
	relPaths := []string{}

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
		relPaths = append(relPaths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}

	slices.Sort(relPaths)

	files := make([]fileEntry, 0, len(relPaths))
	for _, rel := range relPaths {
		entry, err := classifyPolicy(rel, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("classify %s: %w", rel, err)
		}
		files = append(files, entry)
	}

	return files, nil
}

// classifyPolicy determines the translation policy for a single file using
// first-match-wins rules: templates/ paths are preserved as-is, known code
// extensions are copied verbatim, and Markdown files are translated only if
// their body text (excluding frontmatter/code) contains Japanese.
func classifyPolicy(relPath string, fullPath string) (fileEntry, error) {
	if strings.HasPrefix(relPath, "templates/") {
		return fileEntry{Path: relPath, Policy: "preserve"}, nil
	}

	if codeExtensions[filepath.Ext(relPath)] {
		return fileEntry{Path: relPath, Policy: "copy", Reason: "code"}, nil
	}

	if filepath.Ext(relPath) == ".md" {
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return fileEntry{}, err
		}
		if containsJapanese(stripNonBody(string(content))) {
			return fileEntry{Path: relPath, Policy: "translate"}, nil
		}
		return fileEntry{Path: relPath, Policy: "copy", Reason: "english"}, nil
	}

	return fileEntry{Path: relPath, Policy: "copy", Reason: "other"}, nil
}

// stripNonBody removes YAML frontmatter, fenced code blocks, and inline code
// from Markdown content, leaving only prose text for Japanese detection.
func stripNonBody(content string) string {
	content = frontmatterRE.ReplaceAllString(content, "")
	content = fencedCodeRE.ReplaceAllString(content, "")
	content = inlineCodeRE.ReplaceAllString(content, "")
	return content
}

func containsJapanese(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

// listOrphans returns files under skills/<skillName>/ that have no
// corresponding file under skills-jp/<skillName>/, excluding metadata files
// that only ever exist on the skills/ side.
func listOrphans(skillsDir, skillsJPDir, skillName string) ([]string, error) {
	root := filepath.Join(skillsDir, skillName)
	jpRoot := filepath.Join(skillsJPDir, skillName)

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []string{}, nil
	}

	orphans := []string{}

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
		rel = filepath.ToSlash(rel)

		if rel == excludeDoNotEdit || rel == metaFileName {
			return nil
		}

		jpPath := filepath.Join(jpRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(jpPath); os.IsNotExist(err) {
			orphans = append(orphans, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}

	slices.Sort(orphans)
	return orphans, nil
}

func printManifestHelp() {
	fmt.Println(`Output a JSON manifest describing a skill's translation state, files, and orphans.

USAGE:
    translate-meta manifest <skill-name>

OUTPUT:
    A single JSON object on stdout:
        skill    Skill name
        state    up-to-date | needs-update | untranslated
        files    Sorted list of {path, policy, reason} under skills-jp/<skill-name>/
                 policy: preserve | copy | translate
        orphans  Sorted, slash-separated paths under skills/<skill-name>/
                 with no corresponding file under skills-jp/<skill-name>/

EXIT CODES:
    0  success (regardless of which state was found)
    3  error

EXAMPLE:
    translate-meta manifest coding-rules`)
}
