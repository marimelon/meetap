package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop recording",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			log.Fatal("エラー: 録音中のプロセスが見つかりません。")
		}

		var pid int
		fmt.Sscanf(string(data), "%d", &pid)

		process, err := os.FindProcess(pid)
		if err != nil {
			cleanup()
			log.Fatal("プロセスが見つかりません:", err)
		}

		if err := process.Signal(syscall.SIGTERM); err != nil {
			cleanup()
			log.Fatal("停止シグナル送信エラー:", err)
		}

		process.Wait()

		if stateData, err := os.ReadFile(stateFile); err == nil {
			var state recordState
			if json.Unmarshal(stateData, &state) == nil && !quiet {
				fmt.Println("録音停止:")
				printFileInfo(state.SystemFile)
				printFileInfo(state.MicFile)
				printFileInfo(state.MixedFile)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
