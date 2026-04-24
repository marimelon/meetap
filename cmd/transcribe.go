package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/marimelon/meetap/internal/whisper"
	"github.com/spf13/cobra"
)

var (
	modelPath    string
	vadModelPath string
	language     string
	noVAD        bool
	outputFormat string
)

type transcriptOutput struct {
	Timestamp string          `json:"timestamp"`
	Segments  []outputSegment `json:"segments"`
}

type outputSegment struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

type mergedSegment struct {
	Start  time.Duration
	End    time.Duration
	Source string
	Text   string
}

var transcribeCmd = &cobra.Command{
	Use:   "transcribe [timestamp]",
	Short: "Transcribe recorded audio files",
	Long:  "Transcribe system audio and microphone recordings into text. If no timestamp is given, the latest recording is used.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outDir := getOutputDir()

		timestamp := ""
		if len(args) > 0 {
			timestamp = args[0]
		}

		if timestamp == "" {
			timestamp = findLatestTimestamp(outDir)
			if timestamp == "" {
				log.Fatal("録音ファイルが見つかりません。")
			}
			if !quiet {
				fmt.Printf("最新の録音セットを使用: %s\n", timestamp)
			}
		}

		sysWAV := filepath.Join(outDir, fmt.Sprintf("meeting_%s_system.wav", timestamp))
		micWAV := filepath.Join(outDir, fmt.Sprintf("meeting_%s_mic.wav", timestamp))
		outFile := filepath.Join(outDir, fmt.Sprintf("meeting_%s_transcript.%s", timestamp, outputFormat))

		for _, f := range []string{sysWAV, micWAV} {
			if _, err := os.Stat(f); err != nil {
				log.Fatalf("ファイルが見つかりません: %s", f)
			}
		}

		mPath := resolveModelPath(modelPath, "WHISPER_MODEL", "${HOME}/.local/share/whisper-cpp/models/ggml-large-v3.bin")
		vPath := ""
		if !noVAD {
			vPath = resolveModelPath(vadModelPath, "WHISPER_VAD_MODEL", "${HOME}/.local/share/whisper-cpp/models/ggml-silero-vad.bin")
		}

		opts := whisper.TranscribeOptions{
			ModelPath:    mPath,
			Language:     language,
			VADModelPath: vPath,
		}

		if !quiet {
			fmt.Println("文字起こし中...")
		}

		type result struct {
			segments []whisper.Segment
			err      error
		}

		sysCh := make(chan result, 1)
		micCh := make(chan result, 1)

		go func() {
			if !quiet {
				fmt.Printf("  [1/2] system: %s\n", sysWAV)
			}
			segs, err := whisper.Transcribe(sysWAV, opts)
			sysCh <- result{segs, err}
		}()
		go func() {
			if !quiet {
				fmt.Printf("  [2/2] mic:    %s\n", micWAV)
			}
			segs, err := whisper.Transcribe(micWAV, opts)
			micCh <- result{segs, err}
		}()

		sysResult := <-sysCh
		micResult := <-micCh

		if sysResult.err != nil {
			log.Fatal("system 文字起こしエラー:", sysResult.err)
		}
		if micResult.err != nil {
			log.Fatal("mic 文字起こしエラー:", micResult.err)
		}

		merged := mergeSegments(sysResult.segments, "system", micResult.segments, "mic")

		var output []byte
		switch outputFormat {
		case "json":
			output = formatJSON(timestamp, merged)
		default:
			output = formatTxt(merged)
		}

		if err := os.WriteFile(outFile, output, 0644); err != nil {
			log.Fatal("書き込みエラー:", err)
		}

		if !quiet {
			fmt.Printf("  %d segments (system) + %d segments (mic) -> %d merged\n",
				len(sysResult.segments), len(micResult.segments), len(merged))
			fmt.Printf("完了: %s\n", outFile)
		}
	},
}

func init() {
	transcribeCmd.Flags().StringVarP(&modelPath, "model", "m", "", "whisper model path (env: WHISPER_MODEL)")
	transcribeCmd.Flags().StringVar(&vadModelPath, "vad-model", "", "VAD model path (env: WHISPER_VAD_MODEL)")
	transcribeCmd.Flags().StringVarP(&language, "language", "l", "ja", "transcription language")
	transcribeCmd.Flags().BoolVar(&noVAD, "no-vad", false, "disable Voice Activity Detection")
	transcribeCmd.Flags().StringVarP(&outputFormat, "format", "f", "txt", "output format: txt, json")
	rootCmd.AddCommand(transcribeCmd)
}

func resolveModelPath(flag, envKey, defaultPath string) string {
	if flag != "" {
		return flag
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return os.ExpandEnv(defaultPath)
}

func formatTxt(segments []mergedSegment) []byte {
	var lines []string
	for _, seg := range segments {
		ts := formatTimestamp(seg.Start)
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", ts, seg.Source, seg.Text))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func formatJSON(timestamp string, segments []mergedSegment) []byte {
	out := transcriptOutput{
		Timestamp: timestamp,
		Segments:  make([]outputSegment, len(segments)),
	}
	for i, seg := range segments {
		out.Segments[i] = outputSegment{
			Start:  formatTimestamp(seg.Start),
			End:    formatTimestamp(seg.End),
			Source: seg.Source,
			Text:   seg.Text,
		}
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return append(data, '\n')
}

func mergeSegments(sysSegs []whisper.Segment, sysLabel string, micSegs []whisper.Segment, micLabel string) []mergedSegment {
	var all []mergedSegment
	for _, s := range filterHallucinations(sysSegs) {
		all = append(all, mergedSegment{Start: s.Start, End: s.End, Source: sysLabel, Text: s.Text})
	}
	for _, s := range filterHallucinations(micSegs) {
		all = append(all, mergedSegment{Start: s.Start, End: s.End, Source: micLabel, Text: s.Text})
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Start < all[j].Start
	})

	var merged []mergedSegment
	for _, seg := range all {
		if len(merged) > 0 && merged[len(merged)-1].Source == seg.Source {
			gap := seg.Start - merged[len(merged)-1].End
			if gap < 2*time.Second {
				merged[len(merged)-1].Text += " " + seg.Text
				merged[len(merged)-1].End = seg.End
				continue
			}
		}
		merged = append(merged, seg)
	}
	return merged
}

func filterHallucinations(segs []whisper.Segment) []whisper.Segment {
	if len(segs) < 3 {
		return segs
	}
	var filtered []whisper.Segment
	repeatCount := 1
	for i, s := range segs {
		if i > 0 && s.Text == segs[i-1].Text {
			repeatCount++
		} else {
			repeatCount = 1
		}
		if repeatCount <= 2 {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func findLatestTimestamp(outDir string) string {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return ""
	}

	var latest string
	var latestTime time.Time
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "meeting_") && strings.HasSuffix(name, "_system.wav") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latest = strings.TrimPrefix(strings.TrimSuffix(name, "_system.wav"), "meeting_")
			}
		}
	}
	return latest
}

func formatTimestamp(d time.Duration) string {
	s := int(d.Seconds())
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
}
