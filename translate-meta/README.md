# translate-meta

`skills-jp/`（日本語原稿）と `skills/`（英訳）の翻訳追従状態を管理する CLI です。`skills-jp/<skill>/` の内容を `skills/<skill>/.translate-meta`（JSON v2 形式）に記録し、原稿が変わったかを `status` で判定します。`policy: translate` の Markdown ファイルは段落・見出し・コードブロック単位の「セグメント」に分割して追跡するため、`segments status` / `segments assemble` を使うと、変更されたセグメントだけを翻訳し、未変更部分はツールが機械的に再構成できます。

同じコマンド群は `--agents-dir` / `--agents-jp-dir` を指定すると、`agents-jp/<name>.md` ↔ `agents/<name>.md` という単一ファイルの「エージェント」翻訳にもそのまま使えます。詳細は[エージェントモード](#エージェントモード)を参照してください。

## インストール

```bash
go install github.com/yuzu441/tools-go/translate-meta@latest
```

特定バージョンに固定したい場合は `@vX.Y.Z`（例: `@v0.1.0`）でタグを指定できます。

インストール済みのバージョンは `--version` で確認できます。

```bash
$ translate-meta --version
translate-meta v2.0.0
```

バージョンは `internal/cli/version.go` の定数としてツール側に持ちます。monorepo で複数ツールを扱うため、リポジトリ単位の git tag ではツールごとのバージョニングができず、インストールも `@main` 参照を前提としているためです。リリース時はこの定数を手で更新します。

## コマンド一覧

| コマンド | 説明 |
| --- | --- |
| `status <skill-name>` | 翻訳が最新かを判定する |
| `record <skill-name>` | 現在の `skills-jp/<skill-name>/` の状態を `skills/<skill-name>/.translate-meta` に記録する（セグメント対応表の記録・既存 jp/en ペアからのブートストラップを兼ねる） |
| `manifest <skill-name>` | 翻訳状態とファイル一覧（ファイル毎の変更フラグ付き）を JSON で出力する |
| `segments status <skill-name>` | 前回の record 以降に変更されたセグメントの work-list を JSON で出力する（読み取り専用） |
| `segments assemble <skill-name> --patch <file>` | パッチファイルから `policy: translate` ファイルを再構成する |
| `help [<command>]` | 全体 / サブコマンドの help を表示する |

`--version` はコマンドではなくフラグで、指定するとバージョンを表示して終了します（他のコマンドや引数と併記しても、バージョン表示だけを行います）。

各コマンドの `<skill-name>` は、`--agents-dir` / `--agents-jp-dir` を指定した場合は「エージェント名（`.md` を除く）」として扱われます（[エージェントモード](#エージェントモード)参照）。

### `status`

`skills-jp/<skill-name>/` のハッシュと、`skills/<skill-name>/.translate-meta` に保存されたハッシュを比較します。

```bash
translate-meta status coding-rules
```

終了コードで状態を返します。

| 終了コード | 意味 |
| --- | --- |
| `0` | up-to-date（記録と一致） |
| `1` | needs-update（原稿が変更されている） |
| `2` | untranslated（記録ファイルが存在しない） |
| `3` | error |

### `record`

翻訳作業を `skills/<skill-name>/` に反映した後で実行します。`skills-jp/<skill-name>/` の各ファイルを分類し、`skills/<skill-name>/.translate-meta` に JSON（v2 形式）で書き込みます。`policy: translate` の Markdown ファイルは JP/EN 両方をセグメント分割し、セグメント数が一致すれば位置ベースでペアリングして `jp_hash`/`en_hash` を記録します（既存の jp/en ペアからの TM ブートストラップも兼ねます）。セグメント数が一致しない場合はファイル全体のハッシュに degrade し、警告を標準エラーに出力します。旧形式（bare hex）のメタファイルも `record` を実行すれば自動で v2 に移行します。

```bash
translate-meta record coding-rules
```

### `manifest`

`skills-jp/<skill-name>/` の翻訳状態とファイル一覧を JSON で標準出力に出力します。

```bash
translate-meta manifest coding-rules
```

出力例:

```json
{"skill":"coding-rules","state":"up-to-date","files":[{"path":"SKILL.md","policy":"translate","changed":false}],"orphans":[]}
```

| フィールド | 説明 |
| --- | --- |
| `skill` | スキル名 |
| `state` | `up-to-date` / `needs-update` / `untranslated` |
| `files` | `skills-jp/<skill-name>/` 配下のファイル一覧（ソート済み）。各要素は `path` / `policy`（`preserve` \| `copy` \| `translate`）/ `reason`（`copy` の場合のみ）/ `changed` を持つ |
| `orphans` | `skills/<skill-name>/` にのみ存在し、対応する `skills-jp/` 側のファイルが無いパス一覧 |

`changed` は前回の record 以降にそのファイルが変わったかを表します。`policy: preserve` は常に `false`、記録が一度も無いスキルでは（`preserve` を除き）常に `true` です。`status` と異なり、終了コードは成功時 `0` / エラー時 `3` のみを表し、翻訳状態は `state` フィールドで判定します。

### `segments status`

前回の `record` 以降に `policy: translate` ファイルのどのセグメント（見出し・段落・コードブロック単位の翻訳単位）が変わったかを、読み取り専用の work-list として JSON で出力します。未変更セグメントの本文は出力されません（LLM に不要なテキストを渡さないため）。フェンスコードブロックはコメントやプレースホルダを英語に訳す対象のため、行単位ではなくブロック全体が 1 セグメントとして扱われます。

```bash
translate-meta segments status coding-rules
```

出力例:

```json
{
  "skill": "coding-rules",
  "state": "needs-update",
  "files": [
    {
      "path": "SKILL.md",
      "changed": [{"index": 3, "state": "added", "jp_text": "新しい段落です。\n"}],
      "direct_edits": [],
      "unchanged_count": 12
    }
  ]
}
```

| フィールド | 説明 |
| --- | --- |
| `state` | `up-to-date` / `needs-update` / `needs-bootstrap` |
| `files[].changed` | 追加・編集されたセグメント。`index` は `segments assemble` のパッチで使う番号 |
| `files[].direct_edits` | JP は不変だが記録された `en_hash` と現在の英語ファイルの内容が食い違うセグメント（`skills/` への直接編集を検出） |
| `files[].unchanged_count` | 対応が必要ないセグメント数 |

`state` が `needs-bootstrap` の場合、メタファイルが存在しないか旧形式のままでセグメント単位の差分が取れないことを示します。先に `record` を実行してください。

終了コードは `0` (up-to-date) / `1` (needs-update) / `2` (needs-bootstrap) / `3` (error) です。

**移行に関する注意**: コードブロックをセグメント化する以前のバージョンで記録された `.translate-meta` には、コードブロックの `jp_hash`/`en_hash` が存在しません。そのため `segments status` はそれらのコードブロックを常に `added`（追加）として報告します。これは安全側の挙動です（実際には未変更でも翻訳を求め直すだけで、無警告で状態を壊すことはありません）。`record` を再実行すればコードブロックのハッシュが記録され、以降は通常どおり差分だけが報告されます。

### `segments assemble`

パッチファイルから `policy: translate` ファイルを再構成します。未変更セグメントは現在の `skills/<skill-name>/` の内容からバイト単位で保持され、変更されたセグメントだけがパッチの内容に置き換わります。

```bash
translate-meta segments assemble coding-rules --patch /tmp/patch.txt
```

パッチファイルは 1 ファイルにまとめたマーカー区切り形式です。マーカーはファイルパスと `segments status` が返した `index` を指定し、以降の行が生の Markdown 本文になります。

```
=== translate-meta-patch: SKILL.md #3 ===
New English paragraph.
- item 1
- item 2
=== translate-meta-patch: references/example.md #1 ===
Reference text.
```

`segments status` が報告した変更セグメントのいずれかがパッチに無い場合、**何も書き込まずに**エラーになります（不完全な assemble が TM を汚染することを防ぐため）。同じパッチで再実行しても結果は変わりません（冪等）。

未変更（kept）セグメントは現在の `skills/<skill-name>/` からバイト単位でそのまま使われます。これは `segments status` の `direct_edits`（EN 直接編集）が報告されている場合も同様です — assemble は直接編集をブロックしたり上書きしたりしません。ドリフトの検知は `segments status` の責務であり、assemble は現在の英語ファイルを信頼してそのまま採用します。

パッチの各エントリは、assemble が実際に必要とするセグメント（`segments status` が報告した追加セグメント、または再利用可能な英語本文が無い kept セグメント）に対応していなければなりません。ファイル中に実在するセグメント番号であっても、assemble が要求していない番号は拒否されます。セグメント本文中にたまたまマーカー行（`=== translate-meta-patch:` で始まる行）と同じ形式の行が含まれていると、意図せず本物のマーカーと誤認識されることがありますが、これは「要求されていないエントリ」を生み出すため必ず検出されます。該当エントリが assemble の要求と一致しない場合、該当セグメントを静かに切り詰めるのではなく、**何も書き込まずに**エラーになります。

### `help`

```bash
translate-meta help
translate-meta help status
translate-meta help record
translate-meta help manifest
translate-meta help segments
translate-meta help segments status
translate-meta help segments assemble
```

## `.translate-meta` の形式

`record` は `skills/<skill-name>/.translate-meta` に JSON（v2）を書き込みます。

```json
{
  "version": 2,
  "files": {
    "SKILL.md": {
      "policy": "translate",
      "segments": [
        {"jp_hash": "sha256:...", "en_hash": "sha256:..."}
      ]
    },
    "references/example.md": {"policy": "copy", "hash": "sha256:..."},
    "templates/pr_template.md": {"policy": "preserve"}
  }
}
```

英語本文そのものは保存しません（source of truth は `skills/` の実ファイルと git 履歴）。ファイルキーは常にソート済みで、セグメントは 1 行 1 オブジェクトで整形されるため、1 セグメントの変更が git diff 1 行に収まります。

`record` を実行していないスキルや、このバージョン以前に作られた旧形式（64 桁 hex 1 行）のメタファイルも `status` / `manifest` はそのまま読み取れます（旧形式はディレクトリ全体のハッシュ比較で判定）。`record` を実行すると常に v2 形式で書き直されます。

## パス解決の優先順位

参照する 2 つのディレクトリは、以下の優先順位で解決されます。

1. フラグ指定（`--skills-dir` / `--skills-jp-dir`、または `--agents-dir` / `--agents-jp-dir`）
2. デフォルト（`<cwd>/skills` / `<cwd>/skills-jp`、または `<cwd>/agents` / `<cwd>/agents-jp`）

環境変数による上書きはサポートしません。フラグを指定したい場合は、サブコマンドの**前**に置いてください。

```bash
translate-meta --skills-dir ./skills --skills-jp-dir ./skills-jp status coding-rules
```

`--skills-dir` / `--skills-jp-dir` と `--agents-dir` / `--agents-jp-dir` は排他です。両系統のフラグを同時に指定するとエラーになります。

## エージェントモード

`--agents-dir` / `--agents-jp-dir` を指定すると、スキル（ディレクトリ単位）ではなくエージェント（単一ファイル）を対象に同じサブコマンドが動作します。`<skill-name>` は `.md` を除いたエージェント名で、`agents-jp/<name>.md` ↔ `agents/<name>.md` の 1 対 1 に対応します。

```bash
translate-meta --agents-dir ./agents --agents-jp-dir ./agents-jp record reviewer
translate-meta --agents-dir ./agents --agents-jp-dir ./agents-jp segments status reviewer
```

スキルモードとの違いは次の点です。

- **TM の置き場**: `skills/<skill-name>/.translate-meta` ではなく `agents/.translate-meta/<name>.json` に記録されます。`agents/` はエージェントが同じ階層に並ぶ単一の共有ディレクトリのため、エージェントごとのディレクトリを持てません。フォーマットは同じ JSON v2 で、ファイルキーは常に `<name>.md` の 1 エントリだけです。
- **orphan の判定**: `manifest` の `orphans` は、`agents/<name>.md` に対応する `agents-jp/<name>.md` が無いだけでは orphan と報告しません。`agents/` には翻訳対象ではない英語専用のエージェントが正当に存在するためです。orphan と報告されるのは、`agents/.translate-meta/<name>.json`（TM 記録）が存在するのに `agents-jp/<name>.md`（JP 原稿）が消えている場合だけです。共有ファイルの `agents/DO_NOT_EDIT.txt` と `agents/.translate-meta/` 自体は対象になりません。
- 変更検出・セグメント分割・diff・assemble のコアロジックはスキルモードと共通です。ファイル探索先と TM の置き場だけがモードによって切り替わります。

## CI で使うサンプル

monorepo（例: `claude-marketplace-priv` 構成）で、リポジトリ直下の `skills/` と `skills-jp/` を対象に全スキルを判定する場合の例です。

```bash
#!/usr/bin/env bash
set -euo pipefail

SKILLS_DIR="./skills"
SKILLS_JP_DIR="./skills-jp"

for dir in "${SKILLS_JP_DIR}"/*/; do
  skill="$(basename "${dir}")"
  set +e
  translate-meta --skills-dir "${SKILLS_DIR}" --skills-jp-dir "${SKILLS_JP_DIR}" status "${skill}"
  code=$?
  set -e
  case "${code}" in
    0) echo "up-to-date: ${skill}" ;;
    1) echo "needs-update: ${skill}"; exit 1 ;;
    2) echo "untranslated: ${skill}"; exit 1 ;;
    *) echo "error: ${skill}"; exit "${code}" ;;
  esac
done
```
