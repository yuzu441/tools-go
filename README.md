# tools-go

[yuzu441](https://github.com/yuzu441) が個人的に利用する Go 製ツールを 1 リポジトリに集約した monorepo です。

## 方針

- 複数の Go ツールをサブディレクトリ単位で配置する monorepo として運用する。
- 各ツールはサブディレクトリに `main` パッケージを持ち、ツールごとに `go install` できる構成にする。
- ルートには共通ライブラリ（`pkg/` 等）を置かない。共通化は必要が出てから判断する。

## インストール

各ツールのインストールは、ツール単位で行います。

```bash
# 例: <tool> という名前のツールをインストール
go install github.com/yuzu441/tools-go/<tool>@<tag>
```

最新版を試すだけであれば `@latest` でも構いません。

```bash
go install github.com/yuzu441/tools-go/<tool>@latest
```

## 開発

```bash
# モジュール解決
go mod tidy

# ビルド（全パッケージ）
go build ./...

# 静的解析
go vet ./...
```

### lefthook（Git フック）

[lefthook](https://github.com/evilmartians/lefthook) を `go.mod` の `tool` ディレクティブで管理しています。クローン直後に一度だけ以下を実行して Git フックを登録してください。

```bash
go tool lefthook install
```

登録後、`git commit` 時に以下が自動で走ります。

- `gofmt -w`: ステージ済み `*.go` を整形し、修正があれば再 stage
- `go vet ./...`: モジュール全体の静的解析

個別にフックの挙動をローカルで上書きしたい場合は `lefthook-local.yml` を作成してください（`.gitignore` 済み）。

## ライセンス

[MIT License](./LICENSE)
