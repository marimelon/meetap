package cmd

import (
	"encoding/json"
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

	tap, ctx, sysDev, micDev, sysRec, micRec := setupCaptureDevices()
	defer tap.Destroy()
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	startRecording(sysDev, micDev, state)
	waitForStop()

	if !quiet {
		fmt.Println("\n録音停止中...")
	}
	sysDev.Uninit()
	micDev.Uninit()

	saveRecordings(state, sysRec, micRec)
}

func setupRecordState() recordState {
	outDir := getOutputDir()
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
	os.WriteFile(stateFile, stateJSON, 0644)

	return state
}

func setupCaptureDevices() (*coreaudio.Tap, *malgo.AllocatedContext, *malgo.Device, *malgo.Device, *audio.Recorder, *audio.Recorder) {
	format := malgo.FormatS16
	rate := uint32(sampleRate)

	tap, err := coreaudio.CreateGlobalTap()
	if err != nil {
		log.Fatal("Tap作成エラー:", err)
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		tap.Destroy()
		log.Fatal(err)
	}

	captureDevices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		log.Fatal(err)
	}

	var tapDeviceID *malgo.DeviceID
	for _, d := range captureDevices {
		if d.Name() == coreaudio.DeviceName {
			id := d.ID
			tapDeviceID = &id
		}
	}
	if tapDeviceID == nil {
		log.Fatal("CoreAudio Tap デバイスが見つかりません")
	}

	// システム音声 (ステレオ)
	sysRec := &audio.Recorder{}
	sysConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	sysConfig.Capture.DeviceID = tapDeviceID.Pointer()
	sysConfig.Capture.Format = format
	sysConfig.Capture.Channels = 2
	sysConfig.SampleRate = rate

	sysDevice, err := malgo.InitDevice(ctx.Context, sysConfig, malgo.DeviceCallbacks{Data: sysRec.OnData})
	if err != nil {
		log.Fatal("システム音声デバイス初期化エラー:", err)
	}

	// マイク (モノラル)
	micRec := &audio.Recorder{}
	micConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	if micDeviceName != "" {
		var micID *malgo.DeviceID
		for _, d := range captureDevices {
			if d.Name() == micDeviceName {
				id := d.ID
				micID = &id
				break
			}
		}
		if micID == nil {
			var names []string
			for _, d := range captureDevices {
				if d.Name() != coreaudio.DeviceName {
					names = append(names, d.Name())
				}
			}
			log.Fatalf("マイクデバイスが見つかりません: %s\n利用可能: %v", micDeviceName, names)
		}
		micConfig.Capture.DeviceID = micID.Pointer()
	}
	micConfig.Capture.Format = format
	micConfig.Capture.Channels = 1
	micConfig.SampleRate = rate

	micDev, err := malgo.InitDevice(ctx.Context, micConfig, malgo.DeviceCallbacks{Data: micRec.OnData})
	if err != nil {
		log.Fatal("マイクデバイス初期化エラー:", err)
	}

	return tap, ctx, sysDevice, micDev, sysRec, micRec
}

func startRecording(sysDevice, micDev *malgo.Device, state recordState) {
	if err := sysDevice.Start(); err != nil {
		log.Fatal("システム音声開始エラー:", err)
	}
	if err := micDev.Start(); err != nil {
		log.Fatal("マイク開始エラー:", err)
	}

	if !quiet {
		fmt.Printf("録音開始: %s\n", state.StartedAt)
		fmt.Printf("  system: %s\n", state.SystemFile)
		fmt.Printf("  mic:    %s\n", state.MicFile)
	}
}

func waitForStop() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
}

func saveRecordings(state recordState, sysRec, micRec *audio.Recorder) {
	sysData := sysRec.Bytes()
	micData := micRec.Bytes()
	sizeInBytes := uint32(malgo.SampleSizeInBytes(malgo.FormatS16))

	if err := audio.WriteWAV(state.SystemFile, sysData, uint32(sampleRate), 2, sizeInBytes); err != nil {
		fmt.Fprintln(os.Stderr, "system.wav 保存エラー:", err)
	} else if !quiet {
		fmt.Printf("saved: %s (%d bytes)\n", state.SystemFile, len(sysData))
	}

	if err := audio.WriteWAV(state.MicFile, micData, uint32(sampleRate), 1, sizeInBytes); err != nil {
		fmt.Fprintln(os.Stderr, "mic.wav 保存エラー:", err)
	} else if !quiet {
		fmt.Printf("saved: %s (%d bytes)\n", state.MicFile, len(micData))
	}

	mixedData := audio.MixStereoAndMono(sysData, micData)
	if err := audio.WriteWAV(state.MixedFile, mixedData, uint32(sampleRate), 1, sizeInBytes); err != nil {
		fmt.Fprintln(os.Stderr, "mixed.wav 保存エラー:", err)
	} else if !quiet {
		fmt.Printf("saved: %s (%d bytes)\n", state.MixedFile, len(mixedData))
	}
}
