package whisper

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type Segment struct {
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
	Text  string        `json:"text"`
}

type TranscribeOptions struct {
	ModelPath    string
	VADModelPath string
	Language     string
}

// Transcribe はWAVファイルを文字起こしし、セグメント一覧を返す。
func Transcribe(wavPath string, opts TranscribeOptions) ([]Segment, error) {
	model, err := whisper.New(opts.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("model load: %w", err)
	}
	defer model.Close()

	ctx, err := model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("context: %w", err)
	}

	if opts.Language != "" {
		if err := ctx.SetLanguage(opts.Language); err != nil {
			return nil, fmt.Errorf("set language: %w", err)
		}
	}

	if opts.VADModelPath != "" {
		ctx.SetVAD(true)
		ctx.SetVADModelPath(opts.VADModelPath)
	}

	samples, err := loadWAVAsFloat32(wavPath)
	if err != nil {
		return nil, fmt.Errorf("load wav: %w", err)
	}

	if err := ctx.Process(samples, nil, nil, nil); err != nil {
		return nil, fmt.Errorf("process: %w", err)
	}

	var segments []Segment
	for {
		seg, err := ctx.NextSegment()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("next segment: %w", err)
		}
		text := seg.Text
		if text == "" {
			continue
		}
		segments = append(segments, Segment{
			Start: seg.Start,
			End:   seg.End,
			Text:  text,
		})
	}

	return segments, nil
}

// loadWAVAsFloat32 は16bit PCM WAVファイルを読み込み、float32スライス(16kHz mono)として返す。
// whisper.cpp は 16kHz mono を要求する。入力が48kHzの場合はダウンサンプルする。
func loadWAVAsFloat32(path string) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// WAV ヘッダーをパース
	var riffHeader [12]byte
	if _, err := io.ReadFull(f, riffHeader[:]); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}

	var sampleRate uint32
	var channels uint16
	var bitsPerSample uint16

	// チャンクを探す
	for {
		var chunkID [4]byte
		var chunkSize uint32
		if err := binary.Read(f, binary.LittleEndian, &chunkID); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if err := binary.Read(f, binary.LittleEndian, &chunkSize); err != nil {
			return nil, err
		}

		switch string(chunkID[:]) {
		case "fmt ":
			var audioFormat uint16
			binary.Read(f, binary.LittleEndian, &audioFormat)
			binary.Read(f, binary.LittleEndian, &channels)
			binary.Read(f, binary.LittleEndian, &sampleRate)
			var byteRate uint32
			var blockAlign uint16
			binary.Read(f, binary.LittleEndian, &byteRate)
			binary.Read(f, binary.LittleEndian, &blockAlign)
			binary.Read(f, binary.LittleEndian, &bitsPerSample)
			// 残りのfmtチャンクデータをスキップ
			if chunkSize > 16 {
				f.Seek(int64(chunkSize-16), io.SeekCurrent)
			}

		case "data":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(f, data); err != nil {
				return nil, fmt.Errorf("read data: %w", err)
			}

			// int16 → float32 変換
			numSamples := int(chunkSize) / int(bitsPerSample/8)
			raw := make([]float32, numSamples)
			for i := 0; i < numSamples; i++ {
				sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
				raw[i] = float32(sample) / 32768.0
			}

			// ステレオ → モノラル
			if channels == 2 {
				mono := make([]float32, len(raw)/2)
				for i := 0; i < len(mono); i++ {
					mono[i] = (raw[i*2] + raw[i*2+1]) / 2.0
				}
				raw = mono
			}

			// ダウンサンプル (48kHz → 16kHz = 1/3)
			if sampleRate == 48000 {
				resampled := make([]float32, len(raw)/3)
				for i := range resampled {
					resampled[i] = raw[i*3]
				}
				return resampled, nil
			} else if sampleRate == 16000 {
				return raw, nil
			} else if sampleRate == 44100 {
				// 簡易リサンプル 44100 → 16000
				ratio := float64(sampleRate) / 16000.0
				outLen := int(float64(len(raw)) / ratio)
				resampled := make([]float32, outLen)
				for i := range resampled {
					srcIdx := int(float64(i) * ratio)
					if srcIdx < len(raw) {
						resampled[i] = raw[srcIdx]
					}
				}
				return resampled, nil
			}
			return raw, nil

		default:
			f.Seek(int64(chunkSize), io.SeekCurrent)
		}
	}

	return nil, fmt.Errorf("no data chunk found in WAV")
}
