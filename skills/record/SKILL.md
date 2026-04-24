---
name: record
description: macOSでシステム音声+マイクを録音し、文字起こし・解析する（CoreAudio Tap API + whisper.cpp使用、外部依存なし）
user_invocable: true
---

# 会議録音（/record）

macOSでシステム音声とマイクを同時に録音し、文字起こしまで行う。
CoreAudio Tap API + whisper.cpp Go bindings を使用し、単一バイナリで完結する。

## 前提条件

- macOS 14.2以降（CoreAudio Tap API が必要）
- Apple Silicon (arm64)
- whisper モデルファイル（初回のみダウンロード）

## セットアップ

### インストール（Homebrew）

```bash
brew install marimelon/tap/meetap
```

### インストール（GitHub Releases）

```bash
gh release download --repo marimelon/meetap --pattern "meetap-darwin-arm64" --dir ${CLAUDE_PLUGIN_ROOT}
chmod +x ${CLAUDE_PLUGIN_ROOT}/meetap-darwin-arm64
mv ${CLAUDE_PLUGIN_ROOT}/meetap-darwin-arm64 ${CLAUDE_PLUGIN_ROOT}/meetap
```

### アップデート

```bash
meetap self-update
```

### ソースからビルド（Go 1.26+ と cmake が必要）

```bash
cd ${CLAUDE_PLUGIN_ROOT} && make build
```

## whisper モデルのダウンロード

初回のみ必要:
```bash
mkdir -p ~/.local/share/whisper-cpp/models
curl -L -o ~/.local/share/whisper-cpp/models/ggml-large-v3.bin https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin
curl -L -o ~/.local/share/whisper-cpp/models/ggml-silero-vad.bin https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v6.2.0.bin
```

## コマンド

```
meetap start [-d] [--sample-rate N] [--mic NAME]   録音開始（-d でバックグラウンド）
meetap stop                                        録音停止
meetap list [--json]                               録音ファイル一覧
meetap transcribe [timestamp] [flags]              文字起こし
meetap version                                     バージョン情報
meetap self-update                                 最新版に更新
```

### start フラグ

| フラグ | デフォルト | 説明 |
|--------|-----------|------|
| `-d, --daemon` | `false` | バックグラウンドで録音 |
| `--sample-rate` | `48000` | サンプルレート (Hz) |
| `--mic` | システムデフォルト | マイクデバイス名 |
| `--list-devices` | `false` | 利用可能なオーディオデバイスを表示して終了 |

### transcribe フラグ

| フラグ | デフォルト | 説明 |
|--------|-----------|------|
| `-l, --language` | `ja` | 言語 |
| `-m, --model` | `~/.local/share/whisper-cpp/models/ggml-large-v3.bin` | whisper モデルパス |
| `--vad-model` | `~/.local/share/whisper-cpp/models/ggml-silero-vad.bin` | VAD モデルパス |
| `--no-vad` | `false` | VAD を無効化 |
| `-f, --format` | `txt` | 出力形式: `txt`, `json` |

### グローバルフラグ

| フラグ | 説明 |
|--------|------|
| `-o, --output-dir` | 出力ディレクトリ (デフォルト: `/tmp`、env: `RECORD_OUTPUT_DIR`) |
| `-q, --quiet` | 出力を抑制 |

## 出力ファイル

### 録音
- `meeting_YYYYMMDD_HHMMSS_system.wav` — システム音声（ステレオ, 48kHz）
- `meeting_YYYYMMDD_HHMMSS_mic.wav` — マイク音声（モノラル, 48kHz）
- `meeting_YYYYMMDD_HHMMSS_mixed.wav` — ミックス音声（モノラル, RMS ノーマライズ済み）

### 文字起こし
- `meeting_YYYYMMDD_HHMMSS_transcript.txt` or `.json`

## 実行手順

1. `which meetap` でインストール済みか確認。なければ上記のセットアップを案内する
2. ユーザーの要望に応じてコマンドを実行:
   - 録音開始: `meetap start -d`（バックグラウンド推奨）
   - 録音停止: `meetap stop`
   - 文字起こし: `meetap transcribe`
3. 文字起こし結果（`.txt` or `.json`）を読み込み、次の構造で要約する
   - 概要 — 会議の目的と結論（1-3行）
   - 議論のポイント — 主要な論点を箇条書き。発言元（system=相手/mic=自分）を活用し、誰が何を主張したか区別する
   - アクション項目 — 誰が・何を・いつまでに
   - 未解決事項 — 結論が出なかった議題があれば記載

## 注意事項

- デフォルトの出力先は `/tmp` のため OS 再起動で消える。長期保存には `-o` で別ディレクトリを指定する
- 録音中はシステム音声とマイクの両方をキャプチャするため、録音開始前にユーザーへ確認する
