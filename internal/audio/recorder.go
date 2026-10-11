package audio

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// flushInterval はバッファをファイルへ書き出す間隔。
// 異常終了時に失われる録音はこの間隔分に収まる。
const flushInterval = time.Second

// Recorder はオーディオコールバックで受け取ったサンプルを WAV ファイルへ逐次書き出す。
// コールバックはリアルタイムスレッドで呼ばれるため、そこではバッファに貯めるだけにして
// ファイル I/O は別 goroutine で行う。
type Recorder struct {
	mu     sync.Mutex
	buf    []byte
	spare  []byte
	closed bool

	filename string
	w        *WAVWriter
	err      error
	stop     chan struct{}
	done     chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// NewRecorder は filename に WAV ファイルを作成し、定期書き出しを開始する。
func NewRecorder(filename string, sampleRate, channels, bytesPerSample uint32) (*Recorder, error) {
	w, err := CreateWAV(filename, sampleRate, channels, bytesPerSample)
	if err != nil {
		return nil, err
	}
	r := &Recorder{
		filename: filename,
		w:        w,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go r.loop()
	return r, nil
}

// OnData は malgo のキャプチャコールバックとして使う。
func (r *Recorder) OnData(_ []byte, pSample []byte, _ uint32) {
	r.mu.Lock()
	if !r.closed {
		r.buf = append(r.buf, pSample...)
	}
	r.mu.Unlock()
}

func (r *Recorder) loop() {
	defer close(r.done)
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			r.flush()
		case <-r.stop:
			return
		}
	}
}

// flush はバッファの内容をファイルへ追記し、ヘッダのサイズ欄を更新する。
// 書き込みエラーが起きたらその場で stderr に知らせ、以降の書き込みは行わずに Close で返す。
func (r *Recorder) flush() {
	r.mu.Lock()
	data := r.buf
	r.buf = r.spare[:0]
	r.mu.Unlock()

	if len(data) > 0 && r.err == nil {
		_, err := r.w.Write(data)
		if err == nil {
			err = r.w.UpdateHeader()
		}
		if err != nil {
			r.err = err
			fmt.Fprintf(os.Stderr, "録音ファイル書き込みエラー（以降の録音は保存されません）: %s: %v\n", r.filename, err)
		}
	}
	r.spare = data
}

// Size はファイルへ書き出し済みの PCM データのバイト数を返す。Close 後に呼ぶ。
func (r *Recorder) Size() int64 {
	return int64(r.w.dataSize)
}

// Close は以降のサンプルを受け付けないようにし、残りのバッファを書き出してファイルを閉じる。
// 2 回目以降の呼び出しは 1 回目の結果を返す。
func (r *Recorder) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		close(r.stop)
		<-r.done
		r.flush()
		r.closeErr = errors.Join(r.err, r.w.Close())
	})
	return r.closeErr
}
