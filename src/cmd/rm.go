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

	"github.com/containers/toolbox/pkg/podman"
	"github.com/containers/toolbox/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	rmFlags struct {
		deleteAll   bool
		forceDelete bool
	}
)

var rmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove one or more toolbox containers",
	RunE:  rm,
}

func init() {
	flags := rmCmd.Flags()

	flags.BoolVarP(&rmFlags.deleteAll, "all", "a", false, "Remove all toolbox containers.")

	flags.BoolVarP(&rmFlags.forceDelete,
		"force",
		"f",
		false,
		"Force the removal of running and paused toolbox containers.")

	rmCmd.SetHelpFunc(rmHelp)
	rootCmd.AddCommand(rmCmd)
}

func rm(cmd *cobra.Command, args []string) error {
	if utils.IsInsideContainer() {
		if !utils.IsInsideToolboxContainer() {
			return errors.New("this is not a toolbox container")
		}

		if _, err := utils.ForwardToHost(); err != nil {
			return err
		}

		return nil
	}

	if rmFlags.deleteAll {
		logrus.Debug("Fetching containers with label=com.redhat.component=fedora-toolbox")
		args := []string{"--filter", "label=com.redhat.component=fedora-toolbox"}
		containers_old, err := podman.GetContainers(args...)
		if err != nil {
			return errors.New("failed to list containers with com.redhat.component=fedora-toolbox")
		}

		logrus.Debug("Fetching containers with label=com.github.debarshiray.toolbox=true")
		args = []string{"--filter", "label=com.github.debarshiray.toolbox=true"}
		containers_new, err := podman.GetContainers(args...)
		if err != nil {
			return errors.New("failed to list containers with com.github.debarshiray.toolbox=true")
		}

		containers := utils.JoinJSON("ID", containers_old, containers_new)

		for _, container := range containers {
			containerID := container["ID"].(string)
			logrus.Debugf("Deleting container %s", containerID)
			if err := removeContainer(containerID); err != nil {
				if errors.As(err, &podman.ErrRunningContainer) {
					return fmt.Errorf("container %s is running", containerID)
				} else if errors.As(err, &podman.ErrNonExistent) {
					return fmt.Errorf("container %s does not exist", containerID)
				}

				return fmt.Errorf("failed to remove container %s", containerID)
			}
		}
	} else {
		if len(args) == 0 {
			return errors.New("missing argument for \"rm\"\nRun 'toolbox --help' for usage.")
		}

		for _, container := range args {
			logrus.Debugf("Inspecting container %s", container)
			info, err := podman.PodmanInspect("container", container)
			if err != nil {
				return fmt.Errorf("failed to inspect container %s", container)
			}

			var labels map[string]interface{}

			logrus.Debug("Checking if the container is a toolbox container")
			labels, _ = info["Config"].(map[string]interface{})["Labels"].(map[string]interface{})

			if labels["com.redhat.component"] != "fedora-toolbox" &&
				labels["com.github.debarshiray.toolbox"] != "true" {
				return fmt.Errorf("%s is not a toolbox container", container)
			}

			logrus.Debugf("Removing container %s", container)
			err = removeContainer(container)
			if err != nil {
				if errors.As(err, &podman.ErrRunningContainer) {
					return fmt.Errorf("container %s is running", container)
				} else if errors.As(err, &podman.ErrNonExistent) {
					return fmt.Errorf("container %s does not exist", container)
				}

				return fmt.Errorf("failed to remove container %s", container)
			}
		}
	}

	return nil
}

func rmHelp(cmd *cobra.Command, args []string) {
	if err := utils.ShowManual("toolbox-rm"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
}

func removeContainer(container string) error {
	args := []string{"rm"}

	if rmFlags.forceDelete {
		args = append(args, "--force")
	}

	args = append(args, container)

	if err := podman.CmdRun(args...); err != nil {
		return err
	}

	return nil
}
