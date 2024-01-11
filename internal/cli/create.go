/*
   Copyright 2024 Kunal Singh <kunal@kunalsin9h.com>

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

*/

package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gozinc/zinc/version"
	"github.com/spf13/cobra"
)

var (
	noGit bool
)

const (
	templateURL = "https://github.com/gozinc/template.git"
	templGo     = "github.com/a-h/templ/cmd/templ@latest"
	airGo       = "github.com/cosmtrek/air@latest"
)

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().BoolVar(&noGit, "no-git", false, "Whether go use")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Zinc project",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(zincInfoMessage(version.Version, version.GoVersion))
		fmt.Println("")

		projectName := stringPrompt("What's the project name?", "my_app", "my_app")

		startTask("Setting up project files ...")

		projectPath, err := filepath.Abs(projectName)
		logErrorAndExit(err)

		err = os.MkdirAll(projectPath, os.ModePerm)
		logErrorAndExit(err)

		ctx := context.Background()

		gitClone := exec.CommandContext(ctx, "git", "clone", templateURL, ".")
		gitClone.Dir = projectPath

		err = gitClone.Run()
		if err != nil {
			logErrorAndExit(fmt.Errorf("Failed to clone templates"))
		}

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()

			// remove .git folder
			dotGitFolder := path.Join(projectPath, ".git")
			err := os.RemoveAll(dotGitFolder)

			if err != nil {
				logError(err.Error())
				showMessage("Failed to remove .git folder, remove it yourself", true, true)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()

			startTask("Downloading go dependencies ...")

			goModDownload := exec.CommandContext(ctx, "go", "mod", "download")
			goModDownload.Dir = projectPath

			err := goModDownload.Run()
			if err != nil {
				logErrorAndExit(err)
			}

			startTask("Cleaning go project ...")

			tidyCmd := exec.CommandContext(ctx, "go", "mod", "tidy")
			tidyCmd.Dir = projectPath

			err = tidyCmd.Run()
			if err != nil {
				logErrorAndExit(err)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()

			downloadGoTool("templ", templGo, &wg)

			startTask("Generating templ code ...")
			templeGen := exec.CommandContext(ctx, "templ", "generate")
			templeGen.Dir = projectPath

			err := templeGen.Run()
			if err != nil {
				logErrorAndExit(err)
			}
		}()

		wg.Add(1)
		go downloadGoTool("air", airGo, &wg)

		wg.Wait()

		if noGit {
			startTask("Initializing Git")
			err = initializeGitRepo(projectPath)
			if err != nil {
				logError(err.Error())
			}
		}

		if runtime.GOOS == "windows" {
			// changing air config bin path to add .exe
			airTomlFile := path.Join(projectPath, ".air.toml")
			airToml, err := os.ReadFile(airTomlFile)
			if err != nil {
				showMessage(err.Error(), true, true)
			} else {
				airTomlData := string(airToml)
				//                                                     replace in two places
				airTomlData = strings.Replace(airTomlData, "./tmp/main", "./tmp/main.exe", 2)

				err = os.WriteFile(airTomlFile, []byte(airTomlData), os.ModePerm)
				if err != nil {
					showMessage(err.Error(), true, true)
				}
			}
		}

		fmt.Println("")
		downloadTailwind(&wg)

		logSuccess("Done!")

		showMessage("# now run the application", true, false)
		showMessage(fmt.Sprintf("cd %s", projectName), true, false)
		showMessage("zinc run .", true, false)

		return nil
	},
}

func zincInfoMessage(version, goVersion string) string {
	return fmt.Sprintf("%s   v%s, build with Go v%s", cyanBold(zincTextArt()), whiteBold(version), whiteBold(goVersion))
}

func zincTextArt() string {
	return `
▀█ █ █▄░█ █▀▀   Fullstack web framework for golang
█▄ █ █░▀█ █▄▄`
}

func stringPrompt(label, example, defaultValue string) string {
	r := bufio.NewReader(os.Stdin)

	fmt.Fprintf(os.Stdout, "%s  %s %s ", cyanBold("➜"), label, whiteDim(example))

	s, err := r.ReadString('\n')
	if err != nil {
		panic(err)
	}

	s = strings.TrimSpace(s)

	if len(s) == 0 {
		s = defaultValue
	}

	return s
}
