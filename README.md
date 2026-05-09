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

## ライセンス

[MIT License](./LICENSE)
