package coreaudio

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreAudio -framework Foundation
#include "tap.h"
*/
import "C"
import "fmt"

// Tap はシステム音声キャプチャ用の CoreAudio Process Tap を表す。
type Tap struct {
	tapID             C.uint32_t
	aggregateDeviceID C.uint32_t
}

// CreateGlobalTap はシステム全体の音声をキャプチャする Tap を作成する。
// 返される Tap は、aggregate device としてキャプチャデバイス一覧に現れる。
func CreateGlobalTap() (*Tap, error) {
	result := C.createGlobalTap()
	if result.error != 0 {
		return nil, fmt.Errorf("AudioHardwareCreateProcessTap failed (OSStatus %d)", result.error)
	}

	tap := &Tap{
		tapID:             result.tapID,
		aggregateDeviceID: result.aggregateDeviceID,
	}

	var buf [256]C.char
	C.getDeviceName(result.aggregateDeviceID, &buf[0], 256)
	fmt.Printf("Tap aggregate device: %s (ID: %d)\n", C.GoString(&buf[0]), result.aggregateDeviceID)

	return tap, nil
}

// AggregateDeviceID は malgo 等で使える AudioObjectID を返す。
func (t *Tap) AggregateDeviceID() uint32 {
	return uint32(t.aggregateDeviceID)
}

// Destroy は Tap と aggregate device を破棄する。
func (t *Tap) Destroy() {
	C.destroyTap(t.tapID, t.aggregateDeviceID)
}

// DeviceName は "CoreAudio Tap" — malgo のデバイス一覧検索に使う。
const DeviceName = "CoreAudio Tap"
