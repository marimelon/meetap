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

	"github.com/spf13/cobra"
)

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recorded meeting files",
	Run: func(cmd *cobra.Command, args []string) {
		outDir := getOutputDir()

		entries, err := os.ReadDir(outDir)
		if err != nil {
			log.Fatal("ディレクトリ読み取りエラー:", err)
		}

		type fileEntry struct {
			Name    string    `json:"name"`
			Size    int64     `json:"size"`
			ModTime time.Time `json:"mod_time"`
		}

		var files []fileEntry
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "meeting_") && strings.HasSuffix(e.Name(), ".wav") {
				info, err := e.Info()
				if err != nil {
					continue
				}
				files = append(files, fileEntry{
					Name:    filepath.Join(outDir, e.Name()),
					Size:    info.Size(),
					ModTime: info.ModTime(),
				})
			}
		}

		if len(files) == 0 {
			if !listJSON {
				fmt.Println("録音ファイルがありません。")
			} else {
				fmt.Println("[]")
			}
			return
		}

		sort.Slice(files, func(i, j int) bool {
			return files[i].ModTime.After(files[j].ModTime)
		})

		if listJSON {
			data, _ := json.MarshalIndent(files, "", "  ")
			fmt.Println(string(data))
		} else {
			for _, f := range files {
				fmt.Printf("  %s  %s  %s\n", f.ModTime.Format("2006-01-02 15:04:05"), formatSize(f.Size), f.Name)
			}
		}
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(listCmd)
}
