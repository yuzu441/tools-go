package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestIsLegacyMeta(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"64桁hexはlegacy", strings.Repeat("a", 64), true},
		{"JSON形式(先頭{)はlegacyでない", `{"version":2,"files":{}}`, false},
		{"前後空白があってもJSON判定できる", "  {\"version\":2,\"files\":{}}\n", false},
		{"空文字はlegacyでない", "", false},
		{"空白のみはlegacyでない", "   \n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLegacyMeta(tt.content); got != tt.want {
				t.Errorf("isLegacyMeta(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestParseMetaV2(t *testing.T) {
	t.Run("正常なv2 JSONを構造体にパースできる", func(t *testing.T) {
		content := `{"version":2,"files":{"SKILL.md":{"policy":"translate","segments":[{"jp_hash":"sha256:aaa","en_hash":"sha256:bbb"}]},"README.md":{"policy":"copy","hash":"sha256:ccc"},"templates/x.md":{"policy":"preserve"}}}`
		got, err := parseMetaV2(content)
		if err != nil {
			t.Fatalf("parseMetaV2: %v", err)
		}
		want := metaV2{
			Version: 2,
			Files: map[string]fileMeta{
				"SKILL.md":       {Policy: "translate", Segments: []segmentPair{{JPHash: "sha256:aaa", ENHash: "sha256:bbb"}}},
				"README.md":      {Policy: "copy", Hash: "sha256:ccc"},
				"templates/x.md": {Policy: "preserve"},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseMetaV2 = %+v, want %+v", got, want)
		}
	})

	t.Run("不正なJSONはエラー", func(t *testing.T) {
		if _, err := parseMetaV2("{not json"); err == nil {
			t.Error("expected error for malformed JSON")
		}
	})

	t.Run("versionが2以外はエラー", func(t *testing.T) {
		if _, err := parseMetaV2(`{"version":1,"files":{}}`); err == nil {
			t.Error("expected error for unsupported version")
		}
	})
}

func TestMarshalMetaV2_RoundTrip(t *testing.T) {
	m := metaV2{
		Version: 2,
		Files: map[string]fileMeta{
			"z.md": {Policy: "copy", Hash: "sha256:zzz"},
			"SKILL.md": {Policy: "translate", Segments: []segmentPair{
				{JPHash: "sha256:1", ENHash: "sha256:2"},
				{JPHash: "sha256:3", ENHash: "sha256:4"},
			}},
			"templates/t.md": {Policy: "preserve"},
		},
	}

	out, err := marshalMetaV2(m)
	if err != nil {
		t.Fatalf("marshalMetaV2: %v", err)
	}

	got, err := parseMetaV2(string(out))
	if err != nil {
		t.Fatalf("parseMetaV2(marshaled): %v\noutput:\n%s", err, out)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("round trip mismatch:\ngot  %+v\nwant %+v", got, m)
	}
}

func TestMarshalMetaV2_StableKeyOrder(t *testing.T) {
	m := metaV2{Version: 2, Files: map[string]fileMeta{
		"z.md": {Policy: "copy", Hash: "sha256:1"},
		"a.md": {Policy: "copy", Hash: "sha256:2"},
		"m.md": {Policy: "copy", Hash: "sha256:3"},
	}}
	out1, err := marshalMetaV2(m)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := marshalMetaV2(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(out1) != string(out2) {
		t.Fatalf("marshalMetaV2 is not deterministic")
	}

	s := string(out1)
	idxA := strings.Index(s, `"a.md"`)
	idxM := strings.Index(s, `"m.md"`)
	idxZ := strings.Index(s, `"z.md"`)
	if idxA < 0 || idxM < 0 || idxZ < 0 || !(idxA < idxM && idxM < idxZ) {
		t.Errorf("expected sorted key order a.md < m.md < z.md, got offsets %d, %d, %d", idxA, idxM, idxZ)
	}
}

func TestMarshalMetaV2_OneSegmentPerLine(t *testing.T) {
	m := metaV2{Version: 2, Files: map[string]fileMeta{
		"SKILL.md": {Policy: "translate", Segments: []segmentPair{
			{JPHash: "sha256:1", ENHash: "sha256:2"},
			{JPHash: "sha256:3", ENHash: "sha256:4"},
			{JPHash: "sha256:5", ENHash: "sha256:6"},
		}},
	}}
	out, err := marshalMetaV2(m)
	if err != nil {
		t.Fatal(err)
	}

	var segLines []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "jp_hash") {
			segLines = append(segLines, strings.TrimSuffix(strings.TrimSpace(l), ","))
		}
	}
	if len(segLines) != 3 {
		t.Fatalf("expected 3 segment lines, got %d: %v", len(segLines), segLines)
	}
	for _, l := range segLines {
		var seg segmentPair
		if err := json.Unmarshal([]byte(l), &seg); err != nil {
			t.Errorf("segment line is not standalone valid JSON: %q: %v", l, err)
		}
	}
}

func TestComputeFileChanges(t *testing.T) {
	t.Run("未記録のファイルはchanged", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		changes, removed, err := computeFileChanges(cfg, "foo", metaV2{Version: 2, Files: map[string]fileMeta{}})
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 0 {
			t.Errorf("removed = %v, want empty", removed)
		}
		want := []fileChange{{Path: "a.md", Policy: "copy", Changed: true}}
		if !slices.Equal(changes, want) {
			t.Errorf("changes = %v, want %v", changes, want)
		}
	})

	t.Run("copyファイルはハッシュ一致でchanged=false", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "copy", Hash: hashSegment("hello")},
		}}
		changes, _, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if changes[0].Changed {
			t.Errorf("changes[0].Changed = true, want false")
		}
	})

	t.Run("copyファイルはハッシュ不一致でchanged=true", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "copy", Hash: "sha256:deadbeef"},
		}}
		changes, _, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if !changes[0].Changed {
			t.Errorf("changes[0].Changed = false, want true")
		}
	})

	t.Run("preserveファイルは未記録でもchanged=false", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"templates/x.md": "テンプレート"})
		changes, _, err := computeFileChanges(cfg, "foo", metaV2{Version: 2, Files: map[string]fileMeta{}})
		if err != nil {
			t.Fatal(err)
		}
		if changes[0].Changed {
			t.Errorf("changes[0].Changed = true, want false (preserve is never tracked)")
		}
	})

	t.Run("policyが反転していればchanged=true", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "これは日本語です"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "copy", Hash: hashSegment("これは日本語です")},
		}}
		changes, _, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if !changes[0].Changed {
			t.Errorf("changes[0].Changed = false, want true (policy flip copy -> translate)")
		}
	})

	t.Run("translateファイルはsegments未記録ならwhole-fileハッシュで判定", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "これは日本語です"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "translate", Hash: hashSegment("これは日本語です")},
		}}
		changes, _, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if changes[0].Changed {
			t.Errorf("changes[0].Changed = true, want false")
		}
	})

	t.Run("translateファイルはsegments記録があればLCS差分で判定(セグメント不変ならchanged=false)", func(t *testing.T) {
		content := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": content})
		blocks, err := splitSegments(content)
		if err != nil {
			t.Fatal(err)
		}
		segs := translatableSegments(blocks)
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "translate", Segments: []segmentPair{
				{JPHash: hashSegment(segs[0]), ENHash: "sha256:en0"},
				{JPHash: hashSegment(segs[1]), ENHash: "sha256:en1"},
			}},
		}}
		changes, _, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if changes[0].Changed {
			t.Errorf("changes[0].Changed = true, want false")
		}
	})

	t.Run("translateファイルは中間にセグメントが挿入されてもchanged=true", func(t *testing.T) {
		original := "見出し\n\n段落です\n"
		blocks, err := splitSegments(original)
		if err != nil {
			t.Fatal(err)
		}
		origSegs := translatableSegments(blocks)

		edited := "見出し\n\n新しい段落\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": edited})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "translate", Segments: []segmentPair{
				{JPHash: hashSegment(origSegs[0]), ENHash: "sha256:en0"},
				{JPHash: hashSegment(origSegs[1]), ENHash: "sha256:en1"},
			}},
		}}
		changes, _, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if !changes[0].Changed {
			t.Errorf("changes[0].Changed = false, want true (segment inserted)")
		}
	})

	t.Run("記録にあるが現在存在しないファイルはremovedFilesに入る", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md":   {Policy: "copy", Hash: hashSegment("hello")},
			"old.md": {Policy: "copy", Hash: "sha256:whatever"},
		}}
		_, removed, err := computeFileChanges(cfg, "foo", meta)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(removed, "old.md") {
			t.Errorf("removed = %v, want to contain old.md", removed)
		}
	})
}

func TestResolveMetaState(t *testing.T) {
	ctx := t.Context()

	t.Run("メタファイルが無いとUNTRANSLATED", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		resolved, err := resolveMetaState(ctx, cfg, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.State != UNTRANSLATED {
			t.Errorf("State = %d, want %d", resolved.State, UNTRANSLATED)
		}
		if resolved.IsV2 {
			t.Errorf("IsV2 = true, want false")
		}
	})

	t.Run("legacyメタはhashDir比較で状態決定", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		hash, err := hashDir(ctx, filepath.Join(cfg.SkillsJPDir, "foo"))
		if err != nil {
			t.Fatal(err)
		}
		writeMeta(t, cfg, "foo", hash)

		resolved, err := resolveMetaState(ctx, cfg, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.State != UP_TO_DATE {
			t.Errorf("State = %d, want %d (UP_TO_DATE)", resolved.State, UP_TO_DATE)
		}
		if resolved.IsV2 {
			t.Errorf("IsV2 = true, want false for legacy meta")
		}
	})

	t.Run("v2メタで全ファイル一致ならUP_TO_DATE", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "copy", Hash: hashSegment("hello")},
		}}
		writeV2Meta(t, cfg, "foo", meta)

		resolved, err := resolveMetaState(ctx, cfg, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.State != UP_TO_DATE {
			t.Errorf("State = %d, want %d (UP_TO_DATE)", resolved.State, UP_TO_DATE)
		}
		if !resolved.IsV2 {
			t.Errorf("IsV2 = false, want true")
		}
	})

	t.Run("v2メタでファイルが変更されていればNEEDS_UPDATE", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md": {Policy: "copy", Hash: "sha256:stale"},
		}}
		writeV2Meta(t, cfg, "foo", meta)

		resolved, err := resolveMetaState(ctx, cfg, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.State != NEEDS_UPDATE {
			t.Errorf("State = %d, want %d (NEEDS_UPDATE)", resolved.State, NEEDS_UPDATE)
		}
	})

	t.Run("v2メタでファイルが削除されていればNEEDS_UPDATE", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		meta := metaV2{Version: 2, Files: map[string]fileMeta{
			"a.md":   {Policy: "copy", Hash: hashSegment("hello")},
			"old.md": {Policy: "copy", Hash: "sha256:whatever"},
		}}
		writeV2Meta(t, cfg, "foo", meta)

		resolved, err := resolveMetaState(ctx, cfg, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.State != NEEDS_UPDATE {
			t.Errorf("State = %d, want %d (NEEDS_UPDATE)", resolved.State, NEEDS_UPDATE)
		}
	})
}

// writeV2Meta writes a v2 meta struct to skills/<skillName>/.translate-meta,
// mirroring writeMeta's role for legacy content.
func writeV2Meta(t *testing.T, cfg Config, skillName string, meta metaV2) {
	t.Helper()
	out, err := marshalMetaV2(meta)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.SkillsDir, skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaFileName), out, 0o644); err != nil {
		t.Fatal(err)
	}
}
