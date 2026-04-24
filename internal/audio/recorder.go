package audio

import "sync"

// Recorder はオーディオコールバックでサンプルデータを蓄積する。
type Recorder struct {
	mu      sync.Mutex
	samples []byte
}

// OnData は malgo のキャプチャコールバックとして使う。
func (r *Recorder) OnData(_ []byte, pSample []byte, _ uint32) {
	r.mu.Lock()
	r.samples = append(r.samples, pSample...)
	r.mu.Unlock()
}

// Bytes は蓄積されたサンプルデータを返す。
func (r *Recorder) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.samples
}
