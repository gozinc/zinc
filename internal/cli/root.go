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
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/gozinc/zinc/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "zinc",
	Version: version.Version,
	Long:    "Zinc is fullstack Go framework",
}

func ExecuteContext(c context.Context) error {
	return rootCmd.ExecuteContext(c)
}

func init() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

var cyanBold = color.New(color.FgHiCyan).SprintFunc()
var whiteBold = color.New(color.Bold).SprintFunc()
var whiteDim = color.New(color.Faint).SprintFunc()
var redBold = color.New(color.Bold, color.FgHiRed).SprintFunc()
var greenBold = color.New(color.Bold, color.FgHiGreen).SprintFunc()

func logError(msg string) {
	fmt.Printf("%s  Error: %s\n", redBold("✖"), msg)
}

func logSuccess(msg string) {
	fmt.Printf("%s  %s\n", greenBold("✔"), msg)
}

func logErrorAndPanic(err error) {
	if err != nil {
		logError(err.Error())
	}
}
