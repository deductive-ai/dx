package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/deductive-ai/dx/internal/logging"
)

// openBrowser attempts to open the given URL in the user's default browser.
// Returns true if the browser was opened, false if it fell back to printing.
func openBrowser(url string) bool {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		logging.Debug("Unsupported OS for browser open", "os", runtime.GOOS)
		return false
	}

	if err := cmd.Start(); err != nil {
		logging.Debug("Failed to open browser", "error", err)
		return false
	}

	// Don't wait for the browser process to exit
	go func() {
		_ = cmd.Wait()
	}()

	return true
}

// openBrowserOrPrint tries to open the URL in a browser and prints it as fallback.
func openBrowserOrPrint(url string) {
	if openBrowser(url) {
		fmt.Println("  Opening browser...")
	} else {
		fmt.Printf("  Open this URL in your browser:\n  %s\n", url)
	}
}
