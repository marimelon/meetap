package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const wavHeaderSize = 44

// WAVWriter は PCM データを WAV ファイルへ逐次書き込む。
// ヘッダのサイズ欄は UpdateHeader で書き込み済みのデータ量に合わせて更新する。
type WAVWriter struct {
	f        *os.File
	dataSize uint32
}

// CreateWAV はデータ長 0 のヘッダを書いた WAV ファイルを作成する。
func CreateWAV(filename string, sampleRate, channels, bytesPerSample uint32) (*WAVWriter, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	h := make([]byte, wavHeaderSize)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36)
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], uint16(channels))
	binary.LittleEndian.PutUint32(h[24:], sampleRate)
	binary.LittleEndian.PutUint32(h[28:], sampleRate*channels*bytesPerSample)
	binary.LittleEndian.PutUint16(h[32:], uint16(channels*bytesPerSample))
	binary.LittleEndian.PutUint16(h[34:], uint16(bytesPerSample*8))
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], 0)

	if _, err := f.Write(h); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &WAVWriter{f: f}, nil
}

// Write は PCM データをファイル末尾に追記する。
func (w *WAVWriter) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	w.dataSize += uint32(n)
	return n, err
}

// UpdateHeader は RIFF / data チャンクのサイズ欄を現在のデータ量で書き換える。
func (w *WAVWriter) UpdateHeader() error {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], 36+w.dataSize)
	if _, err := w.f.WriteAt(b[:], 4); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(b[:], w.dataSize)
	_, err := w.f.WriteAt(b[:], 40)
	return err
}

// Close はヘッダを確定させてファイルを閉じる。
func (w *WAVWriter) Close() error {
	return errors.Join(w.UpdateHeader(), w.f.Close())
}

// openWAVData は CreateWAV で作った WAV ファイルを開き、
// data チャンク先頭に位置付けたファイルとデータ長を返す。
func openWAVData(filename string) (*os.File, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}
	h := make([]byte, wavHeaderSize)
	if _, err := io.ReadFull(f, h); err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	if string(h[0:4]) != "RIFF" || string(h[8:12]) != "WAVE" || string(h[36:40]) != "data" {
		_ = f.Close()
		return nil, 0, fmt.Errorf("%s: 対応していない WAV 形式です", filename)
	}
	return f, int64(binary.LittleEndian.Uint32(h[40:])), nil
}
