package audio

import "encoding/binary"

// StereoToMono は 16bit ステレオ PCM データをモノラルに変換する（L+R 平均）。
func StereoToMono(data []byte) []byte {
	monoLen := len(data) / 2
	mono := make([]byte, monoLen)
	for i := 0; i < len(data)-3; i += 4 {
		l := int16(binary.LittleEndian.Uint16(data[i:]))
		r := int16(binary.LittleEndian.Uint16(data[i+2:]))
		avg := int16((int32(l) + int32(r)) / 2)
		binary.LittleEndian.PutUint16(mono[i/2:], uint16(avg))
	}
	return mono
}

// BytesToInt16 はリトルエンディアンのバイト列を int16 スライスに変換する。
func BytesToInt16(data []byte) []int16 {
	n := len(data) / 2
	out := make([]int16, n)
	for i := range n {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

// Int16ToBytes は int16 スライスをリトルエンディアンのバイト列に変換する。
func Int16ToBytes(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}
