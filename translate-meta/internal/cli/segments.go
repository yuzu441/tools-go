package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	otelhelper "github.com/yuzu441/tools-go/internal/otel"
)

// changedSegment is one newly-added translatable segment that a translator
// or LLM must produce English text for.
type changedSegment struct {
	Index  int    `json:"index"`
	State  string `json:"state"`
	JPText string `json:"jp_text"`
}

// directEdit is a kept (JP-unchanged) segment whose current English text no
// longer matches what was last recorded, meaning skills/ was hand-edited
// directly without going through record.
type directEdit struct {
	Index  int    `json:"index"`
	ENText string `json:"en_text"`
}

type segmentsFileStatus struct {
	Path           string           `json:"path"`
	Changed        []changedSegment `json:"changed"`
	DirectEdits    []directEdit     `json:"direct_edits"`
	UnchangedCount int              `json:"unchanged_count"`
}

type segmentsStatusOutput struct {
	Skill string               `json:"skill"`
	State string               `json:"state"`
	Files []segmentsFileStatus `json:"files"`
}

// RunSegments dispatches the "segments status" / "segments assemble"
// subcommands.
func RunSegments(ctx context.Context, cfg Config, args []string) int {
	if len(args) == 0 {
		printSegmentsHelp()
		return ERROR
	}

	switch args[0] {
	case "status":
		return RunSegmentsStatus(ctx, cfg, args[1:])
	case "assemble":
		return RunSegmentsAssemble(ctx, cfg, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown segments subcommand: %s\n\n", args[0])
		printSegmentsHelp()
		return ERROR
	}
}

// RunSegmentsStatus prints a read-only work-list of the segments that have
// changed since the last record, without mutating anything. A meta file
// that isn't yet v2 (absent or legacy) cannot be diffed at segment
// granularity, so it is reported as "needs-bootstrap": record must run
// first.
func RunSegmentsStatus(ctx context.Context, cfg Config, args []string) int {
	if len(args) != 1 {
		printSegmentsStatusHelp()
		return ERROR
	}
	skillName := args[0]

	ctx, span := otelhelper.StartSpan(ctx, "cli.segments.status")
	defer span.End()

	resolved, err := resolveMetaState(ctx, cfg, skillName)
	otelhelper.RecordError(span, err)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	if !resolved.IsV2 {
		out := segmentsStatusOutput{Skill: skillName, State: "needs-bootstrap", Files: []segmentsFileStatus{}}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR
		}
		return UNTRANSLATED
	}

	files, err := buildSegmentsFileStatuses(cfg, skillName, resolved.V2)
	if err != nil {
		otelhelper.RecordError(span, err)
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	state := UP_TO_DATE
	for _, f := range files {
		if len(f.Changed) > 0 || len(f.DirectEdits) > 0 {
			state = NEEDS_UPDATE
			break
		}
	}

	out := segmentsStatusOutput{Skill: skillName, State: stateLabel(state), Files: files}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}
	return state
}

func buildSegmentsFileStatuses(cfg Config, skillName string, meta metaV2) ([]segmentsFileStatus, error) {
	jpRoot := filepath.Join(cfg.SkillsJPDir, skillName)
	enRoot := filepath.Join(cfg.SkillsDir, skillName)

	entries, err := listManifestFiles(jpRoot)
	if err != nil {
		return nil, err
	}

	files := make([]segmentsFileStatus, 0, len(entries))
	for _, e := range entries {
		if e.Policy != "translate" {
			continue
		}
		fs, err := buildSegmentsFileStatus(jpRoot, enRoot, e.Path, meta.Files[e.Path])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Path, err)
		}
		files = append(files, fs)
	}
	return files, nil
}

// buildSegmentsFileStatus diffs one file's current JP segments against
// fm's recorded jp_hash sequence (empty if fm was never recorded, which
// naturally reports every current segment as added). For kept segments, it
// separately checks whether the current English text at the recorded
// position still matches the recorded en_hash.
func buildSegmentsFileStatus(jpRoot, enRoot, relPath string, fm fileMeta) (segmentsFileStatus, error) {
	jpContent, err := os.ReadFile(filepath.Join(jpRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return segmentsFileStatus{}, err
	}
	jpBlocks, err := splitSegments(string(jpContent))
	if err != nil {
		return segmentsFileStatus{}, fmt.Errorf("segment jp source: %w", err)
	}
	jpSegs := translatableSegments(jpBlocks)
	ops := diffHashes(recordedJPHashes(fm), hashSegments(jpSegs))

	enSegs, _ := readEnglishSegments(enRoot, relPath)

	fs := segmentsFileStatus{Path: relPath, Changed: []changedSegment{}, DirectEdits: []directEdit{}}
	for _, op := range ops {
		switch op.Kind {
		case opAdded:
			fs.Changed = append(fs.Changed, changedSegment{
				Index: op.NewIndex, State: "added", JPText: jpSegs[op.NewIndex],
			})
		case opKept:
			text, hasEN := lookupKeptText(enSegs, fm, op.OldIndex, op.NewIndex)
			if !hasEN || hashSegment(text) != fm.Segments[op.OldIndex].ENHash {
				fs.DirectEdits = append(fs.DirectEdits, directEdit{Index: op.NewIndex, ENText: text})
				continue
			}
			fs.UnchangedCount++
		}
	}
	return fs, nil
}

func recordedJPHashes(fm fileMeta) []string {
	hashes := make([]string, len(fm.Segments))
	for i, seg := range fm.Segments {
		hashes[i] = seg.JPHash
	}
	return hashes
}

func enSegAt(enSegs []string, i int) (string, bool) {
	if i < 0 || i >= len(enSegs) {
		return "", false
	}
	return enSegs[i], true
}

// lookupKeptText finds the current English text for a kept (JP-unchanged)
// segment. oldIndex is where record last paired it; newIndex is where it
// sits in the current, possibly-since-reshaped file. A prior "segments
// assemble" run (without an intervening record) may have already moved
// this segment's content from oldIndex to newIndex, so oldIndex cannot be
// trusted outright — but comparing aggregate segment counts to guess which
// shape the file is in does not work either: a delete-here/insert-there
// edit can leave the old and new segment counts equal, making the two
// shapes indistinguishable by count alone.
//
// Instead this checks per segment: if the text still at oldIndex hashes to
// what was recorded there (fm.Segments[oldIndex].ENHash), the file has not
// been reshaped since record and oldIndex is authoritative. Otherwise,
// newIndex is used — either because the file was reshaped (in which case
// this is exactly where a correct prior assemble run placed it) or because
// of a direct edit at oldIndex, which is not this function's job to flag
// (see buildSegmentsFileStatus's DirectEdits, which compares the returned
// text's hash against fm.Segments[oldIndex].ENHash itself).
func lookupKeptText(enSegs []string, fm fileMeta, oldIndex, newIndex int) (string, bool) {
	if text, ok := enSegAt(enSegs, oldIndex); ok && hashSegment(text) == fm.Segments[oldIndex].ENHash {
		return text, true
	}
	return enSegAt(enSegs, newIndex)
}

// patchKey identifies one segment addressed by a patch marker line.
type patchKey struct {
	Path  string
	Index int
}

var patchMarkerRE = regexp.MustCompile(`^=== translate-meta-patch: (.+) #(\d+) ===\s*$`)

// parsePatch reads a marker-delimited patch file (see the "segments
// assemble" help text for the format) into a map from (path, index) to raw
// Markdown content, exactly as written between markers.
func parsePatch(content string) (map[patchKey]string, error) {
	lines := splitLines(content)

	patch := make(map[patchKey]string)
	var currentKey patchKey
	var buf strings.Builder
	haveKey := false

	flush := func() error {
		if !haveKey {
			return nil
		}
		if _, exists := patch[currentKey]; exists {
			return fmt.Errorf("duplicate patch entry for %s #%d", currentKey.Path, currentKey.Index)
		}
		patch[currentKey] = buf.String()
		buf.Reset()
		return nil
	}

	for _, line := range lines {
		if m := patchMarkerRE.FindStringSubmatch(stripLineEnding(line)); m != nil {
			if err := flush(); err != nil {
				return nil, err
			}
			index, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, fmt.Errorf("invalid patch marker index %q: %w", m[2], err)
			}
			currentKey = patchKey{Path: m[1], Index: index}
			haveKey = true
			continue
		}
		if !haveKey {
			return nil, fmt.Errorf("patch content before the first marker line")
		}
		buf.WriteString(line)
	}
	if err := flush(); err != nil {
		return nil, err
	}

	if len(patch) == 0 {
		return nil, fmt.Errorf("patch contains no translate-meta-patch markers")
	}
	return patch, nil
}

// parseAssembleArgs splits "segments assemble" arguments into the skill
// name and --patch value. A hand-rolled parser is used instead of the
// stdlib flag package because the skill name (a positional argument)
// precedes --patch, and flag.Parse stops recognizing flags once it sees the
// first non-flag argument.
func parseAssembleArgs(args []string) (skillName, patchPath string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--patch" {
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--patch requires a value")
			}
			patchPath = args[i+1]
			i++
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) != 1 {
		return "", "", fmt.Errorf("expected exactly one skill name, got %d", len(positional))
	}
	if patchPath == "" {
		return "", "", fmt.Errorf("--patch <file> is required")
	}
	return positional[0], patchPath, nil
}

// RunSegmentsAssemble reconstructs every policy:translate file under a
// skill by combining unchanged English segments (byte-preserved from the
// current English file) with newly-translated segments supplied by a
// patch file. It validates completeness before writing anything: if any
// changed segment is missing from the patch, nothing is written and the
// command fails.
func RunSegmentsAssemble(ctx context.Context, cfg Config, args []string) int {
	skillName, patchPath, err := parseAssembleArgs(args)
	if err != nil {
		printSegmentsAssembleHelp()
		return ERROR
	}

	ctx, span := otelhelper.StartSpan(ctx, "cli.segments.assemble")
	defer span.End()

	resolved, err := resolveMetaState(ctx, cfg, skillName)
	otelhelper.RecordError(span, err)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}
	if !resolved.IsV2 {
		slog.ErrorContext(ctx, "skill has no v2 translation-memory record yet; run record first")
		return ERROR
	}

	patchContent, err := os.ReadFile(patchPath)
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}
	patch, err := parsePatch(string(patchContent))
	if err != nil {
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	jpRoot := filepath.Join(cfg.SkillsJPDir, skillName)
	enRoot := filepath.Join(cfg.SkillsDir, skillName)

	entries, err := listManifestFiles(jpRoot)
	if err != nil {
		otelhelper.RecordError(span, err)
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}

	// A line inside a segment's own body can start with the marker prefix and
	// be parsed as a real marker, silently truncating that segment and
	// fabricating a bogus entry. Validating each parsed key against what
	// assembly actually needs turns that into a loud failure.
	requiredKeys, err := requiredPatchKeys(jpRoot, enRoot, entries, resolved.V2)
	if err != nil {
		otelhelper.RecordError(span, err)
		slog.ErrorContext(ctx, err.Error())
		return ERROR
	}
	if invalid := invalidPatchKeys(patch, requiredKeys); len(invalid) > 0 {
		for _, k := range invalid {
			fmt.Fprintf(os.Stderr, "patch entry is not something assembly needs (possible marker collision in a segment body): %s #%d\n", k.Path, k.Index)
		}
		slog.ErrorContext(ctx, "patch contains entries that do not resolve to a current segment; nothing was written")
		return ERROR
	}

	plans := make(map[string]string)
	var missing []patchKey
	for _, e := range entries {
		if e.Policy != "translate" {
			continue
		}
		assembled, miss, err := assembleFile(jpRoot, enRoot, e.Path, resolved.V2.Files[e.Path], patch)
		if err != nil {
			otelhelper.RecordError(span, err)
			slog.ErrorContext(ctx, err.Error())
			return ERROR
		}
		missing = append(missing, miss...)
		if len(miss) == 0 {
			plans[e.Path] = assembled
		}
	}

	if len(missing) > 0 {
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "missing patch content for %s #%d\n", m.Path, m.Index)
		}
		slog.ErrorContext(ctx, "patch is incomplete: nothing was written")
		return ERROR
	}

	for path, content := range plans {
		full := filepath.Join(enRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			slog.ErrorContext(ctx, err.Error())
			return ERROR
		}
	}

	return 0
}

// requiredPatchKeys returns the exact set of (path, index) pairs that
// assembleFile will consult in patch, across every policy:translate file:
// added segments, plus any kept segment for which lookupKeptText finds no
// reusable English content. This is deliberately narrower than "every
// segment index that exists" — a patch entry for an index assembly does
// not need (e.g. a currently-kept segment) is just as much a sign of a
// marker collision as an out-of-range index, since a legitimate work-list
// from "segments status" would never ask for it.
//
// Calling assembleFile with an empty patch computes this for free: every
// key it actually needs comes back as "missing" when nothing is supplied.
func requiredPatchKeys(jpRoot, enRoot string, entries []fileEntry, meta metaV2) (map[patchKey]bool, error) {
	required := make(map[patchKey]bool)
	for _, e := range entries {
		if e.Policy != "translate" {
			continue
		}
		_, missing, err := assembleFile(jpRoot, enRoot, e.Path, meta.Files[e.Path], map[patchKey]string{})
		if err != nil {
			return nil, err
		}
		for _, k := range missing {
			required[k] = true
		}
	}
	return required, nil
}

func invalidPatchKeys(patch map[patchKey]string, valid map[patchKey]bool) []patchKey {
	var invalid []patchKey
	for k := range patch {
		if !valid[k] {
			invalid = append(invalid, k)
		}
	}
	slices.SortFunc(invalid, func(a, b patchKey) int {
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		return cmp.Compare(a.Index, b.Index)
	})
	return invalid
}

// assembleFile reconstructs one translate-policy file's English content.
// Kept segments reuse the current English file's byte-identical content at
// the recorded position; added segments (including ones from a file that
// was never recorded, or whose recorded position has no current English
// counterpart) come from patch. Any addition missing from patch is
// reported in missing without erroring immediately, so RunSegmentsAssemble
// can report every gap across every file before failing as a whole.
func assembleFile(jpRoot, enRoot, relPath string, fm fileMeta, patch map[patchKey]string) (content string, missing []patchKey, err error) {
	jpContent, err := os.ReadFile(filepath.Join(jpRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return "", nil, err
	}
	jpBlocks, err := splitSegments(string(jpContent))
	if err != nil {
		return "", nil, fmt.Errorf("segment jp source %s: %w", relPath, err)
	}
	jpSegs := translatableSegments(jpBlocks)
	ops := diffHashes(recordedJPHashes(fm), hashSegments(jpSegs))

	enSegs, _ := readEnglishSegments(enRoot, relPath)

	replacement := make(map[int]string, len(jpSegs))
	for _, op := range ops {
		if op.Kind != opAdded && op.Kind != opKept {
			continue
		}

		if op.Kind == opKept {
			if text, ok := lookupKeptText(enSegs, fm, op.OldIndex, op.NewIndex); ok {
				replacement[op.NewIndex] = text
				continue
			}
			// No current English content resolvable for this kept segment
			// (neither the recorded nor the already-reshaped layout has it):
			// there is nothing to reuse, so this must come from the patch too.
		}

		text, ok := patch[patchKey{Path: relPath, Index: op.NewIndex}]
		if !ok {
			missing = append(missing, patchKey{Path: relPath, Index: op.NewIndex})
			continue
		}
		replacement[op.NewIndex] = ensureTrailingNewline(text)
	}

	if len(missing) > 0 {
		return "", missing, nil
	}
	return renderWithReplacements(jpBlocks, replacement), nil, nil
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// renderWithReplacements walks blocks in document order, substituting each
// blockText block's content from replacement (keyed by translatable-segment
// index) while copying every other block kind verbatim.
func renderWithReplacements(blocks []block, replacement map[int]string) string {
	var b strings.Builder
	textIndex := 0
	for _, blk := range blocks {
		if blk.Kind == blockText {
			b.WriteString(replacement[textIndex])
			textIndex++
			continue
		}
		b.WriteString(blk.Raw)
	}
	return b.String()
}

func printSegmentsHelp() {
	fmt.Println(`Segment-level translation-memory subcommands.

USAGE:
    translate-meta segments <SUBCOMMAND>

SUBCOMMANDS:
    status      Show which translatable segments changed since the last record
    assemble    Reconstruct translated files from a patch file

EXAMPLE:
    translate-meta segments status coding-rules
    translate-meta help segments status`)
}

func printSegmentsStatusHelp() {
	fmt.Println(`Show a read-only work-list of segments that changed since the last record.

USAGE:
    translate-meta segments status <skill-name>

OUTPUT:
    A single JSON object on stdout:
        skill    Skill name
        state    up-to-date | needs-update | needs-bootstrap
        files    One entry per policy:translate file:
                     path              File path
                     changed           Added segments needing translation:
                                       {index, state: "added", jp_text}
                     direct_edits      Kept segments whose English text was
                                       hand-edited since the last record:
                                       {index, en_text}
                     unchanged_count   Count of segments needing no action

    "needs-bootstrap" means the skill's .translate-meta is missing or still
    in the legacy (pre-v2) format; run "record" first.

EXIT CODES:
    0  up-to-date
    1  needs-update
    2  needs-bootstrap
    3  error

EXAMPLE:
    translate-meta segments status coding-rules`)
}

func printSegmentsAssembleHelp() {
	fmt.Println(`Reconstruct policy:translate files from a patch of changed segments.

USAGE:
    translate-meta segments assemble <skill-name> --patch <file>

PATCH FILE FORMAT:
    One or more marker-delimited sections. Each marker names a file path
    and the segment index (from "segments status") it supplies text for,
    followed by raw Markdown content up to the next marker or end of file:

        === translate-meta-patch: SKILL.md #3 ===
        New English paragraph.
        - item 1
        - item 2
        === translate-meta-patch: references/example.md #1 ===
        Reference text.

DESCRIPTION:
    Every added segment reported by "segments status" must be present in
    the patch. If any is missing, nothing is written and the command
    fails: partial assembly is refused so translation memory is never
    recorded against an incomplete file. Unchanged (kept) segments are
    copied byte-for-byte from the current file under skills/<skill-name>/,
    including any "direct_edits" reported by "segments status": assemble
    trusts the current English file for kept segments and never blocks on
    or overwrites a hand edit. Drift detection is "segments status"'s job,
    not assemble's. Only policy:translate files are touched; copy/preserve
    files are unaffected.

    Every patch entry must be something assembly actually needs (an added
    segment reported by "segments status", or a kept segment with no
    reusable English text) — an entry for any other index, even one that
    is a real segment in the file, is rejected. A line inside a segment's
    own body that happens to start with the marker prefix
    ("=== translate-meta-patch:") can be mistaken for a real marker; since
    that produces exactly such an unrequested entry, assemble fails loudly
    (writing nothing) rather than silently truncating that segment's
    content.

EXIT CODES:
    0  success
    3  error (incomplete patch, patch entry that assembly doesn't need,
       unreadable patch file, or no v2 record yet)

EXAMPLE:
    translate-meta segments assemble coding-rules --patch /tmp/patch.txt`)
}
