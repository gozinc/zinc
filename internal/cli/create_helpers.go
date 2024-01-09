package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
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

func saveFile(directory, fileName, contentURL string) error {
	file := path.Join(directory, fileName)
	_, err := os.Create(file)
	if err != nil {
		return err
	}

	resp, err := http.Get(contentURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return os.WriteFile(file, data, os.ModePerm)
}

func downloadTailwind(wg *sync.WaitGroup) {
	startTask("Downloading tailwind ...")
	cmd := exec.Command("sudo", "npm", "-g", "i", "tailwindcss")
	err := cmd.Run()
	if err != nil {
		showMessage("Failed to download tailwindcss cli using npm", true, true)
		showMessage("Download it yourself", true, true)
	}
	wg.Done()
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
