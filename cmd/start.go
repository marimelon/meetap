package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/marimelon/meetap/internal/audio"
	"github.com/marimelon/meetap/internal/coreaudio"
	"github.com/spf13/cobra"
)

type recordState struct {
	SystemFile string `json:"system_file"`
	MicFile    string `json:"mic_file"`
	MixedFile  string `json:"mixed_file"`
	StartedAt  string `json:"started_at"`
}

var (
	sampleRate  int
	daemon      bool
	micDeviceName string
	listDevices bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start recording system audio and microphone",
	Run: func(cmd *cobra.Command, args []string) {
		if listDevices {
			printDevices()
			return
		}
		if daemon {
			runDaemon()
		} else {
			runForeground()
		}
	},
}

func init() {
	startCmd.Flags().IntVar(&sampleRate, "sample-rate", 48000, "audio sample rate in Hz")
	startCmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "run in background")
	startCmd.Flags().StringVar(&micDeviceName, "mic", "", "microphone device name (default: system default)")
	startCmd.Flags().BoolVar(&listDevices, "list-devices", false, "list available audio devices and exit")
	rootCmd.AddCommand(startCmd)
}

func printDevices() {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	captureDevices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		log.Fatal(err)
	}
	playbackDevices, err := ctx.Devices(malgo.Playback)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Capture devices:")
	for _, d := range captureDevices {
		fmt.Printf("  %s\n", d.Name())
	}
	fmt.Println()
	fmt.Println("Playback devices:")
	for _, d := range playbackDevices {
		fmt.Printf("  %s\n", d.Name())
	}
}

func runDaemon() {
	// 自分自身を --foreground なし（daemon フラグなし）で再起動
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	args := []string{"start", "--sample-rate", fmt.Sprintf("%d", sampleRate)}
	if micDeviceName != "" {
		args = append(args, "--mic", micDeviceName)
	}
	if outputDir != "" {
		args = append([]string{"--output-dir", outputDir}, args...)
	}

	proc := exec.Command(exe, args...)
	proc.Stdout = nil
	proc.Stderr = nil
	proc.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	logFile, err := os.Create("/tmp/meetap_record.log")
	if err != nil {
		log.Fatal(err)
	}
	proc.Stdout = logFile
	proc.Stderr = logFile

	if err := proc.Start(); err != nil {
		log.Fatal("起動失敗:", err)
	}

	// PID ファイルが作成されるまで待つ（最大5秒）
	for range 50 {
		if _, err := os.Stat(pidFile); err == nil {
			if !quiet {
				fmt.Println("録音を開始しました。")
				if data, err := os.ReadFile(stateFile); err == nil {
					fmt.Println(string(data))
				}
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Fprintln(os.Stderr, "録音開始に失敗しました。ログ: /tmp/meetap_record.log")
	os.Exit(1)
}

func runForeground() {
	if _, err := os.Stat(pidFile); err == nil {
		log.Fatal("エラー: 既に録音中です。stop で停止してください。")
	}

	state := setupRecordState()
	defer cleanup()
	// log.Fatal は defer を実行しないため、PID ファイルが残らないよう失敗時は明示的に片付ける
	fail := func(err error) {
		cleanup()
		_ = os.Remove(state.SystemFile)
		_ = os.Remove(state.MicFile)
		log.Fatal(err)
	}

	res, err := setupCaptureDevices(state)
	if err != nil {
		fail(err)
	}
	defer res.cleanup()

	if err := startRecording(res.sysDev, res.micDev, state); err != nil {
		res.cleanup()
		fail(err)
	}
	waitForStop()

	if !quiet {
		fmt.Println("\n録音停止中...")
	}

	// 録音ファイルを閉じた後にサンプルが届かないよう、先にデバイスを止める
	if err := res.sysDev.Stop(); err != nil {
		fmt.Fprintln(os.Stderr, "システム音声停止エラー:", err)
	}
	if err := res.micDev.Stop(); err != nil {
		fmt.Fprintln(os.Stderr, "マイク停止エラー:", err)
	}
	saveRecordings(state, res.sysRec, res.micRec)
}

func setupRecordState() recordState {
	outDir := getOutputDir()
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatal("出力ディレクトリ作成エラー:", err)
	}
	ts := time.Now().Format("20060102_150405")
	state := recordState{
		SystemFile: filepath.Join(outDir, fmt.Sprintf("meeting_%s_system.wav", ts)),
		MicFile:    filepath.Join(outDir, fmt.Sprintf("meeting_%s_mic.wav", ts)),
		MixedFile:  filepath.Join(outDir, fmt.Sprintf("meeting_%s_mixed.wav", ts)),
		StartedAt:  ts,
	}

	if err := os.WriteFile(pidFile, fmt.Appendf(nil, "%d", os.Getpid()), 0644); err != nil {
		log.Fatal("PIDファイル書き込みエラー:", err)
	}

	stateJSON, _ := json.Marshal(state)
	_ = os.WriteFile(stateFile, stateJSON, 0644)

	return state
}

type captureResources struct {
	tap    *coreaudio.Tap
	ctx    *malgo.AllocatedContext
	sysDev *malgo.Device
	micDev *malgo.Device
	sysRec *audio.Recorder
	micRec *audio.Recorder
}

func (r *captureResources) cleanup() {
	if r.sysDev != nil {
		r.sysDev.Uninit()
	}
	if r.micDev != nil {
		r.micDev.Uninit()
	}
	// 通常は saveRecordings で閉じ済み。Close は 2 回目以降は何もしない
	if r.sysRec != nil {
		_ = r.sysRec.Close()
	}
	if r.micRec != nil {
		_ = r.micRec.Close()
	}
	if r.tap != nil {
		r.tap.Destroy()
	}
	if r.ctx != nil {
		_ = r.ctx.Uninit()
		r.ctx.Free()
	}
}

func setupCaptureDevices(state recordState) (_ *captureResources, retErr error) {
	format := malgo.FormatS16
	rate := uint32(sampleRate)
	sizeInBytes := uint32(malgo.SampleSizeInBytes(format))

	res := &captureResources{}
	defer func() {
		if retErr != nil {
			res.cleanup()
		}
	}()

	tap, err := coreaudio.CreateGlobalTap()
	if err != nil {
		return nil, fmt.Errorf("システム音声 Tap 作成エラー: %w", err)
	}
	res.tap = tap

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	res.ctx = ctx

	captureDevices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, err
	}

	tapDeviceID := findDeviceID(captureDevices, coreaudio.DeviceName)
	if tapDeviceID == nil {
		return nil, errors.New("CoreAudio Tap デバイスが見つかりません")
	}

	// システム音声 (ステレオ)
	res.sysRec, err = audio.NewRecorder(state.SystemFile, rate, 2, sizeInBytes)
	if err != nil {
		return nil, fmt.Errorf("system.wav 作成エラー: %w", err)
	}
	sysConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	sysConfig.Capture.DeviceID = tapDeviceID.Pointer()
	sysConfig.Capture.Format = format
	sysConfig.Capture.Channels = 2
	sysConfig.SampleRate = rate

	res.sysDev, err = malgo.InitDevice(ctx.Context, sysConfig, malgo.DeviceCallbacks{Data: res.sysRec.OnData})
	if err != nil {
		return nil, fmt.Errorf("システム音声デバイス初期化エラー: %w", err)
	}

	// マイク (モノラル)
	micConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	if micDeviceName != "" {
		micID := findDeviceID(captureDevices, micDeviceName)
		if micID == nil {
			var names []string
			for _, d := range captureDevices {
				if d.Name() != coreaudio.DeviceName {
					names = append(names, d.Name())
				}
			}
			return nil, fmt.Errorf("マイクデバイスが見つかりません: %s\n利用可能: %v", micDeviceName, names)
		}
		micConfig.Capture.DeviceID = micID.Pointer()
	}
	micConfig.Capture.Format = format
	micConfig.Capture.Channels = 1
	micConfig.SampleRate = rate

	res.micRec, err = audio.NewRecorder(state.MicFile, rate, 1, sizeInBytes)
	if err != nil {
		return nil, fmt.Errorf("mic.wav 作成エラー: %w", err)
	}
	res.micDev, err = malgo.InitDevice(ctx.Context, micConfig, malgo.DeviceCallbacks{Data: res.micRec.OnData})
	if err != nil {
		return nil, fmt.Errorf("マイクデバイス初期化エラー: %w", err)
	}

	return res, nil
}

func findDeviceID(devices []malgo.DeviceInfo, name string) *malgo.DeviceID {
	for _, d := range devices {
		if d.Name() == name {
			id := d.ID
			return &id
		}
	}
	return nil
}

func startRecording(sysDevice, micDev *malgo.Device, state recordState) error {
	if err := sysDevice.Start(); err != nil {
		return fmt.Errorf("システム音声開始エラー: %w", err)
	}
	if err := micDev.Start(); err != nil {
		return fmt.Errorf("マイク開始エラー: %w", err)
	}

	if !quiet {
		fmt.Printf("録音開始: %s\n", state.StartedAt)
		fmt.Printf("  system: %s\n", state.SystemFile)
		fmt.Printf("  mic:    %s\n", state.MicFile)
	}
	return nil
}

func waitForStop() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
}

func saveRecordings(state recordState, sysRec, micRec *audio.Recorder) {
	sysErr := sysRec.Close()
	if sysErr != nil {
		fmt.Fprintln(os.Stderr, "system.wav 保存エラー:", sysErr)
	} else if !quiet {
		fmt.Printf("saved: %s (%d bytes)\n", state.SystemFile, sysRec.Size())
	}

	micErr := micRec.Close()
	if micErr != nil {
		fmt.Fprintln(os.Stderr, "mic.wav 保存エラー:", micErr)
	} else if !quiet {
		fmt.Printf("saved: %s (%d bytes)\n", state.MicFile, micRec.Size())
	}

	if sysErr != nil || micErr != nil {
		return
	}

	if err := audio.MixWAVFiles(state.SystemFile, state.MicFile, state.MixedFile, uint32(sampleRate)); err != nil {
		fmt.Fprintln(os.Stderr, "mixed.wav 保存エラー:", err)
	} else if !quiet {
		fmt.Printf("saved: %s\n", state.MixedFile)
	}
}
