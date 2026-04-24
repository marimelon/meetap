package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	repoOwner = "marimelon"
	repoName  = "meetap"
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update meetap to the latest version",
	Run: func(cmd *cobra.Command, args []string) {
		token := getGitHubToken()

		release, err := fetchLatestRelease(token)
		if err != nil {
			log.Fatal("リリース取得エラー:", err)
		}

		fmt.Printf("Latest release: %s\n", release.TagName)
		fmt.Printf("Current version: %s\n", Version)

		if release.TagName == Version {
			fmt.Println("既に最新版です。")
			return
		}

		assetName := fmt.Sprintf("meetap-darwin-%s", runtime.GOARCH)
		var targetAsset *ghAsset
		for _, a := range release.Assets {
			if a.Name == assetName {
				targetAsset = &a
				break
			}
		}
		if targetAsset == nil {
			log.Fatalf("アセットが見つかりません: %s\nリリースのアセット: %v",
				assetName, assetNames(release.Assets))
		}

		fmt.Printf("Downloading %s...\n", targetAsset.Name)

		exe, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			log.Fatal(err)
		}

		tmpFile := exe + ".new"
		if err := downloadAsset(targetAsset, token, tmpFile); err != nil {
			os.Remove(tmpFile)
			log.Fatal("ダウンロードエラー:", err)
		}

		if err := os.Chmod(tmpFile, 0755); err != nil {
			os.Remove(tmpFile)
			log.Fatal(err)
		}

		// アトミックに置き換え
		oldFile := exe + ".old"
		if err := os.Rename(exe, oldFile); err != nil {
			os.Remove(tmpFile)
			log.Fatal("バイナリ置き換えエラー:", err)
		}
		if err := os.Rename(tmpFile, exe); err != nil {
			// ロールバック
			os.Rename(oldFile, exe)
			log.Fatal("バイナリ置き換えエラー:", err)
		}
		os.Remove(oldFile)

		fmt.Printf("Updated: %s -> %s\n", Version, release.TagName)
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
}

func getGitHubToken() string {
	// 環境変数から取得
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}

	// gh auth token から借用
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		token := strings.TrimSpace(string(out))
		if token != "" {
			return token
		}
	}

	return ""
}

func fetchLatestRelease(token string) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadAsset(asset *ghAsset, token, destPath string) error {
	req, err := http.NewRequest("GET", asset.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Downloaded %s (%s)\n", asset.Name, formatSize(written))
	return nil
}

func assetNames(assets []ghAsset) []string {
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	return names
}
