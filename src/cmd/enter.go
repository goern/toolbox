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
	enterFlags struct {
		container string
		release   int
	}
)

var enterCmd = &cobra.Command{
	Use:   "enter",
	Short: "Enter a toolbox container for interactive use",
	RunE:  enter,
}

func init() {
	flags := enterCmd.Flags()

	flags.StringVarP(&enterFlags.container,
		"container",
		"c",
		"",
		"Enter a toolbox container with the given name.")

	flags.IntVarP(&enterFlags.release,
		"release",
		"r",
		-1,
		"Enter a toolbox container for a different operating system release than the host.")

	enterCmd.SetHelpFunc(enterHelp)
	rootCmd.AddCommand(enterCmd)
}

func enter(cmd *cobra.Command, args []string) error {
	return nil
}

func enterHelp(cmd *cobra.Command, args []string) {
	if err := utils.ShowManual("toolbox-enter"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
}
