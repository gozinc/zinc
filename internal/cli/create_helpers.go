package cli

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/jmorganca/ollama/progress"
)

func initializeGitRepo(dir string, p *progress.Progress) error {
	status := "Initializing Git"
	s := progress.NewSpinner(status)
	p.Add(status, s)
	defer s.Stop()

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
