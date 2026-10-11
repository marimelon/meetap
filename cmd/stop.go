package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

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
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
			cleanup()
			log.Fatal("PIDファイルの解析エラー:", err)
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			cleanup()
			log.Fatal("プロセスが見つかりません:", err)
		}

		// state ファイルは録音プロセスが終了時に消すため、停止前に読んでおく
		stateData, stateErr := os.ReadFile(stateFile)

		if err := process.Signal(syscall.SIGTERM); err != nil {
			cleanup()
			log.Fatal("停止シグナル送信エラー:", err)
		}

		waitForRecorderExit(process)

		if stateErr == nil {
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

// waitForRecorderExit は録音プロセスが保存を終えるまで待つ。
// 録音プロセスは stop の子プロセスではないので Wait は使えない。
// 保存が終わると PID ファイルが消えるので、それを完了の合図にする。
func waitForRecorderExit(process *os.Process) {
	for {
		if _, err := os.Stat(pidFile); os.IsNotExist(err) {
			return
		}
		// PID ファイルを残したままプロセスが終わっていたら、代わりに片付ける
		if err := process.Signal(syscall.Signal(0)); err != nil {
			cleanup()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
