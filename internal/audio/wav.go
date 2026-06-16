package audio

import (
	"encoding/binary"
	"io"
	"os"
)

// WriteWAV は PCM データを WAV ファイルとして書き出す。
func WriteWAV(filename string, data []byte, sampleRate, channels, bytesPerSample uint32) (retErr error) {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); retErr == nil {
			retErr = closeErr
		}
	}()

	dataSize := uint32(len(data))
	bitsPerSample := bytesPerSample * 8
	byteRate := sampleRate * channels * bytesPerSample
	blockAlign := channels * bytesPerSample

	w := &errWriter{w: f}
	w.write([]byte("RIFF"))
	w.writeLE(uint32(36 + dataSize))
	w.write([]byte("WAVE"))

	w.write([]byte("fmt "))
	w.writeLE(uint32(16))
	w.writeLE(uint16(1))
	w.writeLE(uint16(channels))
	w.writeLE(sampleRate)
	w.writeLE(byteRate)
	w.writeLE(uint16(blockAlign))
	w.writeLE(uint16(bitsPerSample))

	w.write([]byte("data"))
	w.writeLE(dataSize)
	w.write(data)

	return w.err
}

type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) write(p []byte) {
	if ew.err != nil {
		return
	}
	_, ew.err = ew.w.Write(p)
}

func (ew *errWriter) writeLE(v any) {
	if ew.err != nil {
		return
	}
	ew.err = binary.Write(ew.w, binary.LittleEndian, v)
}
