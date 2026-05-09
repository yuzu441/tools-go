# translate-meta

`skills-jp/`（日本語原稿）と `skills/`（英訳）の翻訳追従状態を管理する CLI です。`skills-jp/<skill>/` の内容ハッシュを `skills/<skill>/.translate-meta` に記録し、原稿が変わったかを `status` で判定します。

## インストール

```bash
go install github.com/yuzu441/tools-go/translate-meta@latest
```

特定バージョンに固定したい場合は `@vX.Y.Z`（例: `@v0.1.0`）でタグを指定できます。

## コマンド一覧

| コマンド | 説明 |
| --- | --- |
| `status <skill-name>` | 翻訳が最新かを判定する |
| `record <skill-name>` | 現在の `skills-jp/<skill-name>/` のハッシュを `skills/<skill-name>/.translate-meta` に記録する |
| `help [<command>]` | 全体 / サブコマンドの help を表示する |

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

翻訳作業を `skills/<skill-name>/` に反映した後で実行します。`skills-jp/<skill-name>/` のハッシュを `skills/<skill-name>/.translate-meta` に書き込みます。

```bash
translate-meta record coding-rules
```

### `help`

```bash
translate-meta help
translate-meta help status
translate-meta help record
```

## パス解決の優先順位

参照する 2 つのディレクトリは、以下の優先順位で解決されます。

1. フラグ指定（`--skills-dir` / `--skills-jp-dir`）
2. デフォルト（`<cwd>/skills` / `<cwd>/skills-jp`）

環境変数による上書きはサポートしません。フラグを指定したい場合は、サブコマンドの**前**に置いてください。

```bash
translate-meta --skills-dir ./skills --skills-jp-dir ./skills-jp status coding-rules
```

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
