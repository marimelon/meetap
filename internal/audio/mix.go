package audio

import (
	"fmt"
	"math"
)

const (
	// mixHeadroomRatio はミックス時のヘッドルーム比率。
	// 大きい方の RMS のこの割合をターゲットにすることでクリッピングを防ぐ。
	mixHeadroomRatio = 0.7
)

// MixStereoAndMono はステレオのシステム音声とモノラルのマイク音声を
// RMS ノーマライズしてモノラルにミックスする。
func MixStereoAndMono(stereoData, monoData []byte) []byte {
	sysMono := StereoToMono(stereoData)

	sysSamples := BytesToInt16(sysMono)
	micSamples := BytesToInt16(monoData)

	n := min(len(sysSamples), len(micSamples))

	sysRMS := calcRMS(sysSamples[:n])
	micRMS := calcRMS(micSamples[:n])

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

	fmt.Printf("mix: sysRMS=%.0f micRMS=%.0f sysGain=%.2f micGain=%.2f\n", sysRMS, micRMS, sysGain, micGain)

	out := make([]int16, n)
	for i := range n {
		mixed := float64(sysSamples[i])*sysGain + float64(micSamples[i])*micGain
		out[i] = int16(max(math.MinInt16, min(math.MaxInt16, int(mixed))))
	}

	return Int16ToBytes(out)
}

func calcRMS(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(samples)))
}
