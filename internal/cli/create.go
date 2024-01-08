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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gozinc/zinc/version"
	"github.com/spf13/cobra"
)

var (
	noGit bool
)

const (
	airTomlURL      = "https://raw.githubusercontent.com/gozinc/zinc/main/internal/cli/template/.air.toml"
	gitIgnoreURL    = "https://raw.githubusercontent.com/gozinc/zinc/main/internal/cli/template/.gitignore"
	tailwindConfURL = "https://raw.githubusercontent.com/gozinc/zinc/main/internal/cli/template/tailwind.config.js"
	tailwindSource  = "https://raw.githubusercontent.com/tailwindlabs/tailwindcss/master/src/css/preflight.css"
	htmxSource      = "https://unpkg.com/htmx.org@1.9.10/dist/htmx.min.js"
	airGo           = "github.com/cosmtrek/air@latest"
	templGo         = "github.com/a-h/templ/cmd/templ@latest"
)

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().BoolVar(&noGit, "no-git", false, "Whether go use")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Zinc project",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(zincInfoMessage(version.Version))
		fmt.Println("")

		projectName := stringPrompt("What's the project name?", "my_app", "my_app")
		tailwind := stringPrompt("Will you be using Tailwind CSS for styling", "yes", "yes")
		htmx := stringPrompt("Will you use HTMX?", "yes", "yes")

		startTask("Setting up files ...")

		projectPath, err := filepath.Abs(projectName)
		logErrorAndPanic(err)

		err = os.MkdirAll(projectPath, os.ModePerm)
		logErrorAndPanic(err)

		staticDir := path.Join(projectPath, "static")

		if tailwind != "no" && tailwind != "n" {
			err = saveFile(projectPath, "tailwind.config.js", tailwindConfURL)
			logErrorAndPanic(err)

			cssDir := path.Join(staticDir, "css")
			err = os.MkdirAll(cssDir, os.ModePerm)
			logErrorAndPanic(err)

			err = saveFile(cssDir, "tailwind.css", tailwindSource)
			logErrorAndPanic(err)

			logSuccess("Setup Tailwind CSS")
		}

		if htmx != "no" && htmx != "n" {
			jsDir := path.Join(staticDir, "js")
			err = os.MkdirAll(jsDir, os.ModePerm)
			logErrorAndPanic(err)

			err = saveFile(jsDir, "htmx.min.js", htmxSource)
			logErrorAndPanic(err)

			logSuccess("Setup HTMX")
		}

		err = saveFile(projectPath, ".air.toml", airTomlURL)
		logErrorAndPanic(err)

		if !noGit {
			err = saveFile(projectPath, ".gitignore", gitIgnoreURL)
			logErrorAndPanic(err)

			err = initializeGitRepo(projectPath)
			logErrorAndPanic(err)

			logSuccess("Setup Git")
		}

		startTask("Downloading tools ...")

		var wg sync.WaitGroup
		wg.Add(3)

		downloadTailwind(&wg)
		go downloadGoTool("air", airGo, &wg)
		go downloadGoTool("templ", templGo, &wg)

		wg.Wait()
		logSuccess("Done!")

		fmt.Printf(`
	# now run the application
	cd %s
	zinc run . # not yet
`, projectName)
		return nil
	},
}

func zincInfoMessage(version string) string {
	return fmt.Sprintf("%s   v%s", cyanBold(zincTextArt()), whiteBold(version))
}

func zincTextArt() string {
	return `
▀█ █ █▄░█ █▀▀   fullstack web framework for golang
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
