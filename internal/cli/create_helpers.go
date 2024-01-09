package cli

import (
	"fmt"
	"os/exec"
	"sync"
)

func initializeGitRepo(dir string) error {
	if !isGitInstalled(dir) {
		logError("Git is not installed. Skipping Git initialization")
		return nil
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = dir

	err := cmd.Run()
	logErrorAndExit(err)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir

	err = cmd.Run()
	logErrorAndExit(err)

	return nil
}

func isGitInstalled(dir string) bool {
	cmd := exec.Command("git", "--version")
	cmd.Dir = dir

	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

// TODO: download Tailwindcss with this function
func downloadTailwind(wg *sync.WaitGroup) {
	showMessage("Download tailwindcss cli, preferably using npm, do:", false, false)
	showMessage("npm -g i tailwindcss", true, false)
}

func downloadGoTool(name, src string, wg *sync.WaitGroup) {
	startTask(fmt.Sprintf("Downloading %s ...\n", name))

	cmd := exec.Command("go", "install", src)
	err := cmd.Run()
	if err != nil {
		showMessage(fmt.Sprintf("Failed to download %s using go install\n", name), true, true)
		showMessage("Download it yourself", true, true)
	}

	wg.Done()
}
