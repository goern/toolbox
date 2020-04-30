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
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/containers/toolbox/pkg/podman"
	"github.com/containers/toolbox/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	cgroupsVersion int

	currentUser *user.User

	currentUserShell string

	executable string

	executableBase string

	rootCmd = &cobra.Command{
		Use:               "toolbox",
		Short:             "Unprivileged development environment",
		PersistentPreRunE: preRun,
	}

	rootFlags struct {
		assumeYes bool
		logLevel  string
		logPodman bool
		verbose   bool
	}

	workingDirectory string
)

func Execute() {
	if err := setUpGlobals(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

func init() {
	persistentFlags := rootCmd.PersistentFlags()

	persistentFlags.BoolVarP(&rootFlags.assumeYes,
		"assumeyes",
		"y",
		false,
		"Automatically answer yes for all questions.")

	persistentFlags.StringVar(&rootFlags.logLevel,
		"log-level",
		"error",
		"Log messages at the specified level: trace, debug, info, warn, error, fatal or panic.")

	persistentFlags.BoolVar(&rootFlags.logPodman,
		"log-podman",
		false,
		"Show the log output of Podman. The log level is handled by the log-level option.")

	persistentFlags.BoolVarP(&rootFlags.verbose, "verbose", "v", false, "Set log-level to 'debug'.")

	rootCmd.SetHelpFunc(rootHelp)
	rootCmd.SetUsageFunc(rootUsage)
}

func preRun(cmd *cobra.Command, args []string) error {
	cmd.Root().SilenceUsage = true

	if err := setUpLoggers(); err != nil {
		return err
	}

	podman.SetLogLevel(rootFlags.logLevel)

	logrus.Debugf("Running as real user ID %s", currentUser.Uid)
	logrus.Debugf("Resolved absolute path to the executable as %s", executable)

	if !utils.IsInsideContainer() {
		logrus.Debugf("Running on a cgroups v%d host", cgroupsVersion)
	}

	return nil
}

func rootHelp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing command\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "create    Create a new toolbox container\n")
		fmt.Fprintf(os.Stderr, "enter     Enter an existing toolbox container\n")
		fmt.Fprintf(os.Stderr, "list      List all existing toolbox containers and images\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Run 'toolbox --help' for usage.\n")
	} else {
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				break
			}

			if arg == "--help" || arg == "-h" {
				if utils.IsInsideContainer() {
					if !utils.IsInsideToolboxContainer() {
						fmt.Fprintf(os.Stderr, "Error: this is not a toolbox container\n")
						return
					}

					if _, err := utils.ForwardToHost(); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %s\n", err)
						return
					}
				} else {
					if err := utils.ShowManual("toolbox"); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %s\n", err)
						return
					}
				}
			}
		}
	}
}

func rootUsage(cmd *cobra.Command) error {
	err := errors.New("Run 'toolbox --help' for usage.")
	fmt.Fprintf(os.Stderr, "%s", err)
	return err
}

func setUpGlobals() error {
	var err error

	if !utils.IsInsideContainer() {
		cgroupsVersion, err = utils.GetCgroupsVersion()
		if err != nil {
			return errors.New("failed to get the cgroups version")
		}
	}

	currentUser, err = user.Current()
	if err != nil {
		return errors.New("failed to get the current user")
	}

	currentUserShell = os.Getenv("SHELL")
	if currentUserShell == "" {
		return errors.New("failed to get the current user's default shell")
	}

	executable, err = os.Executable()
	if err != nil {
		return errors.New("failed to get the path to the executable")
	}

	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return errors.New("failed to resolve absolute path to the executable")
	}

	executableBase = filepath.Base(executable)

	workingDirectory, err = os.Getwd()
	if err != nil {
		return errors.New("failed to get the working directory")
	}

	return nil
}

func setUpLoggers() error {
	logrus.SetOutput(os.Stderr)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
	})

	if rootFlags.verbose {
		rootFlags.logLevel = "debug"
	}

	logLevel, err := logrus.ParseLevel(rootFlags.logLevel)
	if err != nil {
		return errors.New("failed to parse log-level")
	}

	logrus.SetLevel(logLevel)
	return nil
}
