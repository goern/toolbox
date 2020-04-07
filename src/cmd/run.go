/*
 * Copyright © 2019 – 2020 Red Hat Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"fmt"
	"os"

	"github.com/containers/toolbox/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	runFlags struct {
		container string
		release   int
	}
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a command in an existing toolbox container",
	RunE:  run,
}

func init() {
	flags := runCmd.Flags()
	flags.SetInterspersed(false)

	flags.StringVarP(&runFlags.container,
		"container",
		"c",
		"",
		"Run command inside a toolbox container with the given name.")

	flags.IntVarP(&runFlags.release,
		"release",
		"r",
		-1,
		"Run command inside a toolbox container for a different operating system release than the host.")

	runCmd.SetHelpFunc(runHelp)
	rootCmd.AddCommand(runCmd)
}

func run(cmd *cobra.Command, args []string) error {
	return nil
}

func runHelp(cmd *cobra.Command, args []string) {
	if err := utils.ShowManual("toolbox-run"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
}
