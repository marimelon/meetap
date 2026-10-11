package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	// mixHeadroomRatio はミックス時のヘッドルーム比率。
	// 大きい方の RMS のこの割合をターゲットにすることでクリッピングを防ぐ。
	mixHeadroomRatio = 0.7

	// mixChunkFrames はミックス時に一度に読み込むフレーム数。
	mixChunkFrames = 48000
)

// MixWAVFiles はステレオのシステム音声とモノラルのマイク音声の WAV ファイルを
// RMS ノーマライズしてモノラルにミックスし、outPath に書き出す。
// 録音全体の RMS でゲインを決めるため、入力を 2 回読む。
func MixWAVFiles(stereoPath, monoPath, outPath string, sampleRate uint32) (retErr error) {
	sysF, sysSize, err := openWAVData(stereoPath)
	if err != nil {
		return err
	}
	defer func() { _ = sysF.Close() }()

	micF, micSize, err := openWAVData(monoPath)
	if err != nil {
		return err
	}
	defer func() { _ = micF.Close() }()

	frames := min(sysSize/4, micSize/2)

	var sysSum, micSum float64
	err = forEachMixChunk(sysF, micF, frames, func(sys, mic []int16) error {
		for i := range sys {
			sysSum += float64(sys[i]) * float64(sys[i])
			micSum += float64(mic[i]) * float64(mic[i])
		}
		return nil
	})
	if err != nil {
		return err
	}

	var sysRMS, micRMS float64
	if frames > 0 {
		sysRMS = math.Sqrt(sysSum / float64(frames))
		micRMS = math.Sqrt(micSum / float64(frames))
	}

	targetRMS := math.Max(sysRMS, micRMS) * mixHeadroomRatio
	if targetRMS < 1 {
		targetRMS = 1
	}

	sysGain := 1.0
	micGain := 1.0
	if sysRMS > 0 {
		sysGain = targetRMS / sysRMS
	}
	if micRMS > 0 {
		micGain = targetRMS / micRMS
	}

	if _, err := sysF.Seek(wavHeaderSize, io.SeekStart); err != nil {
		return err
	}
	if _, err := micF.Seek(wavHeaderSize, io.SeekStart); err != nil {
		return err
	}

	out, err := CreateWAV(outPath, sampleRate, 1, 2)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, out.Close()) }()

	outBuf := make([]byte, mixChunkFrames*2)
	return forEachMixChunk(sysF, micF, frames, func(sys, mic []int16) error {
		for i := range sys {
			m := float64(sys[i])*sysGain + float64(mic[i])*micGain
			binary.LittleEndian.PutUint16(outBuf[i*2:], uint16(int16(max(math.MinInt16, min(math.MaxInt16, int(m))))))
		}
		_, err := out.Write(outBuf[:len(sys)*2])
		return err
	})
}

// forEachMixChunk はシステム音声 (ステレオ) とマイク (モノラル) を先頭から frames フレーム分、
// チャンクごとにモノラルの int16 サンプルとして fn に渡す。
// fn に渡すスライスは呼び出しをまたいで使い回す。
func forEachMixChunk(sysR, micR io.Reader, frames int64, fn func(sys, mic []int16) error) error {
	sysBuf := make([]byte, mixChunkFrames*4)
	micBuf := make([]byte, mixChunkFrames*2)
	sys := make([]int16, mixChunkFrames)
	mic := make([]int16, mixChunkFrames)
	for frames > 0 {
		n := min(frames, mixChunkFrames)
		if _, err := io.ReadFull(sysR, sysBuf[:n*4]); err != nil {
			return err
		}
		if _, err := io.ReadFull(micR, micBuf[:n*2]); err != nil {
			return err
		}
		for i := range n {
			l := int16(binary.LittleEndian.Uint16(sysBuf[i*4:]))
			r := int16(binary.LittleEndian.Uint16(sysBuf[i*4+2:]))
			sys[i] = int16((int32(l) + int32(r)) / 2)
			mic[i] = int16(binary.LittleEndian.Uint16(micBuf[i*2:]))
		}
		if err := fn(sys[:n], mic[:n]); err != nil {
			return err
		}
		frames -= n
	}
	return nil
}
