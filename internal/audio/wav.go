package audio

import (
	"encoding/binary"
	"os"
)

// WriteWAV は PCM データを WAV ファイルとして書き出す。
func WriteWAV(filename string, data []byte, sampleRate, channels, bytesPerSample uint32) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	dataSize := uint32(len(data))
	bitsPerSample := bytesPerSample * 8
	byteRate := sampleRate * channels * bytesPerSample
	blockAlign := channels * bytesPerSample

	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(36+dataSize))
	f.Write([]byte("WAVE"))

	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16))
	binary.Write(f, binary.LittleEndian, uint16(1))
	binary.Write(f, binary.LittleEndian, uint16(channels))
	binary.Write(f, binary.LittleEndian, sampleRate)
	binary.Write(f, binary.LittleEndian, byteRate)
	binary.Write(f, binary.LittleEndian, uint16(blockAlign))
	binary.Write(f, binary.LittleEndian, uint16(bitsPerSample))

	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, dataSize)
	f.Write(data)

	return nil
}
