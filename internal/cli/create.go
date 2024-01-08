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
	"path/filepath"
	"strings"

	"github.com/gozinc/zinc/version"
	"github.com/jmorganca/ollama/progress"
	"github.com/spf13/cobra"
)

var (
	noGit bool
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
		tailwind := stringPrompt("Will you be using Tailwind CSS for styling", "y/n", "")
		htmx := stringPrompt("Will you use HTMX?", "y/n", "")

		projectPath, err := filepath.Abs(projectName)
		logErrorAndPanic(err)

		err = os.MkdirAll(projectPath, os.ModePerm)
		logErrorAndPanic(err)

		p := progress.NewProgress(os.Stdout)
		defer p.Stop()

		//

		if !noGit {
			err = initializeGitRepo(projectPath, p)
			logErrorAndPanic(err)
		}
		return nil
	},
}

func zincInfoMessage(version, goVersion string) string {
	return fmt.Sprintf("%s   v%s build with Go %s", cyanBold(zincTextArt()), whiteBold(version), whiteBold(goVersion))
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
