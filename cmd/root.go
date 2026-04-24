package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	pidFile  = "/tmp/meetap_record.pid"
	stateFile = "/tmp/meetap_record.json"
)

var (
	outputDir string
	quiet     bool
)

var rootCmd = &cobra.Command{
	Use:   "meetap",
	Short: "macOS system audio + microphone recorder with transcription",
	Long:  "Record system audio and microphone simultaneously using CoreAudio Tap API, then transcribe with whisper.cpp.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output-dir", "o", "", "output directory (default: /tmp, env: RECORD_OUTPUT_DIR)")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress output")
}

func getOutputDir() string {
	if outputDir != "" {
		return outputDir
	}
	if d := os.Getenv("RECORD_OUTPUT_DIR"); d != "" {
		return d
	}
	return "/tmp"
}

func cleanup() {
	os.Remove(pidFile)
	os.Remove(stateFile)
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func printFileInfo(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	fmt.Printf("  %s (%s)\n", path, formatSize(info.Size()))
}
