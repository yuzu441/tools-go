package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const metaVersion = 2

// segmentPair records the hash of one translatable segment's Japanese source
// and its paired English translation. ENHash is empty until a translation
// has been assembled for that segment.
type segmentPair struct {
	JPHash string `json:"jp_hash"`
	ENHash string `json:"en_hash,omitempty"`
}

// fileMeta records the translation-memory state of one file under a skill.
// Exactly one of Hash or Segments is populated depending on policy: "copy"
// files carry a whole-file Hash, "translate" files carry Segments, and
// "preserve" files carry neither.
type fileMeta struct {
	Policy   string        `json:"policy"`
	Hash     string        `json:"hash,omitempty"`
	Segments []segmentPair `json:"segments,omitempty"`
}

// metaV2 is the structured, JSON-formatted successor to the legacy bare-hash
// .translate-meta content. Files is keyed by the file's path relative to the
// skill root.
type metaV2 struct {
	Version int                 `json:"version"`
	Files   map[string]fileMeta `json:"files"`
}

// isLegacyMeta reports whether raw .translate-meta content is the pre-v2
// format: a bare hash digest rather than a JSON object. Empty content is
// reported as not-legacy; callers handle the empty case separately.
func isLegacyMeta(content string) bool {
	trimmed := strings.TrimSpace(content)
	return trimmed != "" && !strings.HasPrefix(trimmed, "{")
}

func parseMetaV2(content string) (metaV2, error) {
	var m metaV2
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return metaV2{}, fmt.Errorf("parse meta v2: %w", err)
	}
	if m.Version != metaVersion {
		return metaV2{}, fmt.Errorf("unsupported meta version: %d", m.Version)
	}
	return m, nil
}

// marshalMetaV2 renders m as JSON with a stable file-key order and one
// segment object per line, so that editing a single segment produces a
// single-line git diff.
func marshalMetaV2(m metaV2) ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"version\": %d,\n", m.Version)
	if err := writeFilesField(&b, m.Files); err != nil {
		return nil, err
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func writeFilesField(b *strings.Builder, files map[string]fileMeta) error {
	b.WriteString("  \"files\": {")

	keys := slices.Sorted(maps.Keys(files))
	if len(keys) == 0 {
		b.WriteString("}\n")
		return nil
	}

	b.WriteString("\n")
	for i, k := range keys {
		if err := writeFileEntry(b, k, files[k]); err != nil {
			return fmt.Errorf("encode file entry %q: %w", k, err)
		}
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }\n")
	return nil
}

func writeFileEntry(b *strings.Builder, path string, fm fileMeta) error {
	keyJSON, err := json.Marshal(path)
	if err != nil {
		return err
	}
	policyJSON, err := json.Marshal(fm.Policy)
	if err != nil {
		return err
	}

	fmt.Fprintf(b, "    %s: {\n", keyJSON)
	fmt.Fprintf(b, "      \"policy\": %s", policyJSON)

	switch {
	case len(fm.Segments) > 0:
		if err := writeSegmentsField(b, fm.Segments); err != nil {
			return err
		}
	case fm.Hash != "":
		hashJSON, err := json.Marshal(fm.Hash)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ",\n      \"hash\": %s\n", hashJSON)
	default:
		b.WriteString("\n")
	}

	b.WriteString("    }")
	return nil
}

func writeSegmentsField(b *strings.Builder, segments []segmentPair) error {
	b.WriteString(",\n      \"segments\": [\n")
	for j, seg := range segments {
		segJSON, err := json.Marshal(seg)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "        %s", segJSON)
		if j < len(segments)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("      ]\n")
	return nil
}

// resolvedMetaState is the result of reading a target's meta file once and
// classifying it as absent, legacy, or v2. Both translationState and
// RunManifest build on this so the meta file is only read and hashed once
// per invocation.
type resolvedMetaState struct {
	State int
	V2    metaV2
	IsV2  bool
}

// resolveMetaState determines a target's translation state. Legacy (pre-v2)
// content is compared as a single whole-target hash, since that format
// records nothing finer; v2 content is compared file by file.
func resolveMetaState(ctx context.Context, cfg Config, skillName string) (resolvedMetaState, error) {
	metaPath := cfg.MetaPath(skillName)
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return resolvedMetaState{State: UNTRANSLATED}, nil
	}

	content, err := readMetaFile(metaPath)
	if err != nil {
		return resolvedMetaState{}, err
	}
	// Empty meta content: treat as needs-update so the caller forces a re-record.
	if strings.TrimSpace(content) == "" {
		return resolvedMetaState{State: NEEDS_UPDATE}, nil
	}

	if isLegacyMeta(content) {
		hash, err := legacyHash(ctx, cfg, skillName)
		if err != nil {
			return resolvedMetaState{}, err
		}
		state := UP_TO_DATE
		if hash != strings.TrimSpace(content) {
			state = NEEDS_UPDATE
		}
		return resolvedMetaState{State: state}, nil
	}

	meta, err := parseMetaV2(content)
	if err != nil {
		return resolvedMetaState{}, err
	}
	changes, removed, err := computeFileChanges(cfg, skillName, meta)
	if err != nil {
		return resolvedMetaState{}, err
	}
	state := UP_TO_DATE
	if len(removed) > 0 || anyFileChanged(changes) {
		state = NEEDS_UPDATE
	}
	return resolvedMetaState{State: state, V2: meta, IsV2: true}, nil
}

func anyFileChanged(changes []fileChange) bool {
	for _, c := range changes {
		if c.Changed {
			return true
		}
	}
	return false
}

// fileChange describes whether one file currently on the Japanese side
// differs from what is recorded in a v2 meta.
type fileChange struct {
	Path    string
	Policy  string
	Changed bool
}

// computeFileChanges classifies every file currently on the Japanese side
// against meta, and separately reports removedFiles: paths recorded in meta
// that no longer exist on disk.
func computeFileChanges(cfg Config, skillName string, meta metaV2) (changes []fileChange, removedFiles []string, err error) {
	jpRoot := cfg.JPRoot(skillName)

	entries, err := cfg.ListFiles(skillName)
	if err != nil {
		return nil, nil, err
	}

	seen := make(map[string]bool, len(entries))
	changes = make([]fileChange, 0, len(entries))
	for _, e := range entries {
		seen[e.Path] = true
		fm, recorded := meta.Files[e.Path]
		changed, err := isFileChanged(jpRoot, e, fm, recorded)
		if err != nil {
			return nil, nil, fmt.Errorf("check %s: %w", e.Path, err)
		}
		changes = append(changes, fileChange{Path: e.Path, Policy: e.Policy, Changed: changed})
	}

	for path := range meta.Files {
		if !seen[path] {
			removedFiles = append(removedFiles, path)
		}
	}
	slices.Sort(removedFiles)

	return changes, removedFiles, nil
}

// isFileChanged reports whether one file differs from its recorded fileMeta
// fm. recorded is false when e.Path has no entry in meta.Files at all.
func isFileChanged(jpRoot string, e fileEntry, fm fileMeta, recorded bool) (bool, error) {
	if e.Policy == "preserve" {
		// preserve files are never source-tracked for translation.
		return false, nil
	}
	if !recorded {
		return true, nil
	}
	if fm.Policy != e.Policy {
		// A policy flip (e.g. an english reference gained Japanese text)
		// invalidates whatever was previously recorded for this path.
		return true, nil
	}

	switch e.Policy {
	case "copy":
		return isWholeFileChanged(jpRoot, e.Path, fm.Hash)
	case "translate":
		if len(fm.Segments) > 0 {
			return isSegmentedFileChanged(jpRoot, e.Path, fm.Segments)
		}
		return isWholeFileChanged(jpRoot, e.Path, fm.Hash)
	default:
		return true, fmt.Errorf("unknown policy %q", e.Policy)
	}
}

func isWholeFileChanged(jpRoot, relPath, recordedHash string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(jpRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return false, err
	}
	return hashSegment(string(content)) != recordedHash, nil
}

// isSegmentedFileChanged reports whether the current file's translatable
// segments differ from recorded.
func isSegmentedFileChanged(jpRoot, relPath string, recorded []segmentPair) (bool, error) {
	content, err := os.ReadFile(filepath.Join(jpRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return false, err
	}
	blocks, err := splitSegments(string(content))
	if err != nil {
		return false, fmt.Errorf("segment %s: %w", relPath, err)
	}
	currentHashes := hashSegments(translatableSegments(blocks))

	recordedHashes := make([]string, len(recorded))
	for i, seg := range recorded {
		recordedHashes[i] = seg.JPHash
	}

	for _, op := range diffHashes(recordedHashes, currentHashes) {
		if op.Kind != opKept {
			return true, nil
		}
	}
	return false, nil
}
