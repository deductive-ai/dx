package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/deductive-ai/dx/internal/color"
	"github.com/deductive-ai/dx/internal/version"
	"github.com/spf13/cobra"
)

const githubRepo = "deductive-ai/dx"

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade dx to the latest version",
	Long: `Check for a newer version of dx and install it.

Downloads the latest release from GitHub and replaces the current binary.

Examples:
  dx upgrade`,
	Run: runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	version.SuppressHint = len(os.Args) >= 2 && os.Args[1] == "upgrade"
}

func runUpgrade(cmd *cobra.Command, args []string) {
	if Version == "dev" {
		fmt.Fprintf(os.Stderr, "%s This is a development build. Upgrade manually with go install or git pull.\n", color.Error("✗"))
		os.Exit(1)
	}

	fmt.Printf("  Current version: %s\n", color.Info(Version))
	fmt.Print("  Checking for updates... ")

	latest, err := fetchLatestVersion()
	if err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		os.Exit(1)
	}

	latestClean := strings.TrimPrefix(latest, "v")
	fmt.Println(color.Success("✓"))

	if version.CompareVersions(Version, latestClean) >= 0 {
		fmt.Printf("\n  %s dx %s is already the latest version.\n", color.Success("✓"), Version)
		return
	}

	fmt.Printf("  Latest version:  %s\n", color.Info(latestClean))
	fmt.Println()
	fmt.Printf("  Downloading dx %s... ", latestClean)

	archiveName := fmt.Sprintf("dx_%s_%s_%s.tar.gz", latestClean, runtime.GOOS, runtime.GOARCH)
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", githubRepo, latestClean, archiveName)

	tmpDir, err := os.MkdirTemp("", "dx-upgrade-*")
	if err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Error creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	newBinary := filepath.Join(tmpDir, "dx")
	if err := downloadAndExtract(downloadURL, newBinary); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(color.Success("✓"))

	currentBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error finding current binary: %v\n", err)
		os.Exit(1)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error resolving binary path: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Installing to %s... ", currentBinary)

	if err := replaceBinary(newBinary, currentBinary); err != nil {
		fmt.Println(color.Error("✗"))
		fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "\n  To install manually:\n")
		fmt.Fprintf(os.Stderr, "    sudo cp %s %s\n", newBinary, currentBinary)
		os.Exit(1)
	}
	fmt.Println(color.Success("✓"))

	fmt.Printf("\n  %s Upgraded dx %s → %s\n", color.Success("✓"), Version, latestClean)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func fetchLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("no releases found")
	}

	return release.TagName, nil
}

func downloadAndExtract(url, destBinary string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d (check that %s/%s is a supported platform)", resp.StatusCode, runtime.GOOS, runtime.GOARCH)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read archive: %w", err)
		}

		if filepath.Base(hdr.Name) == "dx" && hdr.Typeflag == tar.TypeReg {
			out, err := os.OpenFile(destBinary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("failed to create binary: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("failed to write binary: %w", err)
			}
			_ = out.Close()
			return nil
		}
	}

	return fmt.Errorf("dx binary not found in archive")
}

func replaceBinary(newPath, currentPath string) error {
	// Try direct rename (works if same filesystem and writable)
	if err := os.Rename(newPath, currentPath); err == nil {
		return nil
	}

	// Try copy (handles cross-filesystem)
	src, err := os.Open(newPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return trySudoMove(newPath, currentPath)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func trySudoMove(newPath, currentPath string) error {
	cmd := exec.Command("sudo", "cp", newPath, currentPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

