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
	"fmt"

	"github.com/gozinc/zinc/version"
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
		return nil
	},
}

func zincInfoMessage(version, goVersion string) string {
	return fmt.Sprintf("%s   v%s build with Go %s", cyanBold(zincTextArt()), whiteBold(version), whiteBold(goVersion))
}

func zincTextArt() string {
	return `
▀█ █ █▄░█ █▀▀
█▄ █ █░▀█ █▄▄`
}
