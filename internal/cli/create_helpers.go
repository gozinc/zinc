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
	logErrorAndPanic(err)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir

	err = cmd.Run()
	logErrorAndPanic(err)

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

func downloadTailwind(wg *sync.WaitGroup) error {
	startTask("Downloading tailwind ...")
	cmd := exec.Command("sudo", "npm", "-g", "i", "tailwindcss")
	err := cmd.Run()
	fmt.Println("	Failed to download tailwindcss cli using npm")
	fmt.Println("	Download it yourself")
	logErrorAndPanic(err)
	wg.Done()
	return nil
}

func downloadGoTool(name, src string, wg *sync.WaitGroup) error {
	startTask(fmt.Sprintf("Downloading %s ...\n", name))
	cmd := exec.Command("go", "install", src)
	err := cmd.Run()
	fmt.Println(fmt.Sprintf("	Failed to download %s using npm\n", name))
	fmt.Println("	Download it yourself")
	logErrorAndPanic(err)
	wg.Done()
	return nil
}
