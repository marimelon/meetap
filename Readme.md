# meetap

macOS でシステム音声とマイクを同時に録音し、whisper.cpp で文字起こしする CLI ツール。

- CoreAudio Tap API でシステム音声をキャプチャ（BlackHole 等の仮想デバイス不要）
- whisper.cpp Go bindings で文字起こし（VAD 対応、外部コマンド不要）
- 単一バイナリで完結
- Claude Code skill としても利用可能

## 必要環境

- macOS 14.2 以降（Sonoma）
- Apple Silicon (arm64)

## インストール

### Homebrew

```bash
brew install marimelon/tap/meetap
```

### GitHub Releases から取得

```bash
gh release download --repo marimelon/meetap --pattern "meetap-darwin-arm64" --output meetap
chmod +x meetap
sudo mv meetap /usr/local/bin/  # または任意のパスに配置
```

### ソースからビルド

Go 1.26+ と cmake が必要です。

```bash
git clone https://github.com/marimelon/meetap.git
cd meetap
make build
```

初回ビルド時に whisper.cpp (v1.8.4) をクローン・ビルドするため数分かかります。

## whisper モデルのセットアップ

初回のみ必要です。

```bash
mkdir -p ~/.local/share/whisper-cpp/models

# whisper large-v3 モデル (3.1GB)
curl -L -o ~/.local/share/whisper-cpp/models/ggml-large-v3.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin

# Silero VAD モデル (864KB)
curl -L -o ~/.local/share/whisper-cpp/models/ggml-silero-vad.bin \
  https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v6.2.0.bin
```

## 使い方

### 録音

```bash
# フォアグラウンドで録音（Ctrl+C で停止）
meetap start

# バックグラウンドで録音
meetap start -d

# 停止
meetap stop
```

録音すると 3 つの WAV ファイルが生成されます。

| ファイル | 内容 |
|---------|------|
| `meeting_*_system.wav` | システム音声（ステレオ, 48kHz） |
| `meeting_*_mic.wav` | マイク音声（モノラル, 48kHz） |
| `meeting_*_mixed.wav` | ミックス（モノラル, RMS ノーマライズ済み） |

### 文字起こし

```bash
# 最新の録音を文字起こし
meetap transcribe

# タイムスタンプを指定
meetap transcribe 20260425_012343

# JSON 形式で出力
meetap transcribe -f json

# 英語で文字起こし
meetap transcribe -l en

# VAD を無効化
meetap transcribe --no-vad
```

文字起こしは system と mic を個別に処理し、タイムスタンプで時系列にマージします。

```
[00:00:00] system: それでは会議を始めましょう
[00:00:05] mic: はい、よろしくお願いします
[00:00:08] system: まず前回のアクション項目の確認ですが
```

- `system` — スピーカーから聞こえる音声（会議の他の参加者等）
- `mic` — マイクで拾った音声（自分の発言）

### その他

```bash
# 録音ファイル一覧
meetap list
meetap list --json

# バージョン確認
meetap version

# 最新版に更新
meetap self-update
```

## コマンドリファレンス

```
meetap start [-d] [--sample-rate N]     録音開始
meetap stop                             録音停止
meetap list [--json]                    録音ファイル一覧
meetap transcribe [timestamp] [flags]   文字起こし
meetap version                          バージョン情報
meetap self-update                      最新版に更新
```

### グローバルフラグ

| フラグ | 説明 |
|--------|------|
| `-o, --output-dir` | 出力ディレクトリ（デフォルト: `/tmp`、env: `RECORD_OUTPUT_DIR`） |
| `-q, --quiet` | 出力を抑制 |

### transcribe フラグ

| フラグ | デフォルト | 説明 |
|--------|-----------|------|
| `-l, --language` | `ja` | 言語 |
| `-m, --model` | `~/.local/share/whisper-cpp/models/ggml-large-v3.bin` | whisper モデルパス |
| `--vad-model` | `~/.local/share/whisper-cpp/models/ggml-silero-vad.bin` | VAD モデルパス |
| `--no-vad` | `false` | VAD を無効化 |
| `-f, --format` | `txt` | 出力形式: `txt`, `json` |

### 環境変数

| 変数 | 説明 |
|------|------|
| `RECORD_OUTPUT_DIR` | 出力ディレクトリ |
| `WHISPER_MODEL` | whisper モデルパス |
| `WHISPER_VAD_MODEL` | VAD モデルパス |
| `GITHUB_TOKEN` / `GH_TOKEN` | `self-update` 時の認証（API レート制限回避） |

## Claude Code skill として使う

このリポジトリは Claude Code plugin としても利用できます。

```
/plugin marketplace add marimelon/meetap
```

`/record` コマンドで録音・文字起こし・要約が行えます。

## アーキテクチャ

```
meetap
├── main.go                     CLI エントリポイント
├── cmd/                        cobra コマンド定義
│   ├── root.go                 グローバルフラグ
│   ├── start.go                録音開始（フォアグラウンド/デーモン）
│   ├── stop.go                 録音停止
│   ├── list.go                 ファイル一覧
│   ├── transcribe.go           文字起こし
│   ├── selfupdate.go           自己更新
│   └── version.go              バージョン表示
├── internal/
│   ├── audio/                  WAV 書き出し、ミックス、変換
│   ├── coreaudio/              CoreAudio Tap API (cgo/Objective-C)
│   └── whisper/                whisper.cpp Go bindings ラッパー
├── .claude-plugin/             Claude Code plugin マニフェスト
├── skills/record/SKILL.md      Claude Code skill 定義
├── .goreleaser.yml             GoReleaser 設定
└── Makefile                    ビルド（whisper.cpp 静的ライブラリ含む）
```
