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

package podman

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/HarryMichal/go-version"
	"github.com/sirupsen/logrus"
)

var (
	// ErrNonExistent signals one of the specified containers does not exist (applies to `podman rm/rmi`)
	ErrNonExistent = errors.New("exit status 1")
	// ErrRunningContainer signals one of the specified containers is paused or running (applies to `podman rm`)
	ErrRunningContainer = errors.New("exit status 2")
	// ErrHasChildren signals one of the specified images has child images or is used by a container (applies to `podman rmi`)
	ErrHasChildren = errors.New("exit status 2")
	// ErrInternal signals an error in Podman itself
	ErrInternal = errors.New("exit status 125")
	// ErrCmdCantInvoke signals a contained command cannot be invoked (applies to `podman run/exec`)
	ErrCmdCantInvoke = errors.New("exit status 126")
	// ErrCmdNotFound signals a contained command cannot be found (applies to `podman run/exec`)
	ErrCmdNotFound = errors.New("exit status 127")
	// ErrServiceUnavailable signals a problem while pulling an image from a registry (applies to `podman pull`)
	ErrServiceUnavailable = errors.New("invalid status code from registry 503 (Service Unavailable)")
	// ErrConnectionRefused signals the host machine is probably not online (applies to `podman pull`)
	ErrConnectionRefused = errors.New("connection refused")
	// ErrUnknownManifest signals the requested image does not exist (applies to `podman pull`)
	ErrUnknownManifest = errors.New("manifest unknown")

	LogLevel = "error"
)

func IsPathBindMount(path string, containerInfo map[string]interface{}) bool {
	containerMounts := containerInfo["Mounts"].([]interface{})
	for _, mount := range containerMounts {
		dest := fmt.Sprint(mount.(map[string]interface{})["Destination"])
		if dest == path {
			return true
		}
	}

	return false
}

// CheckVersion compares provided version with the version of Podman.
//
// Takes in one string parameter that should be in the format that is used for versioning (eg. 1.0.0, 2.5.1-dev).
//
// Returns true if the Podman version is equal to or higher than the required version.
func CheckVersion(requiredVersion string) bool {
	podmanVersion, _ := GetVersion()

	podmanVersion = version.Normalize(podmanVersion)
	requiredVersion = version.Normalize(requiredVersion)

	return version.CompareSimple(podmanVersion, requiredVersion) >= 0
}

// GetVersion returns version of Podman in a string
func GetVersion() (string, error) {
	args := []string{"version", "-f", "json"}
	output, err := CmdOutput(args...)
	if err != nil {
		return "", err
	}

	var jsonoutput map[string]interface{}
	err = json.Unmarshal(output, &jsonoutput)
	if err != nil {
		return "", err
	}

	var podmanVersion string
	podmanClientInfoInterface := jsonoutput["Client"]
	switch podmanClientInfo := podmanClientInfoInterface.(type) {
	case nil:
		podmanVersion = jsonoutput["Version"].(string)
	case map[string]interface{}:
		podmanVersion = podmanClientInfo["Version"].(string)
	}
	return podmanVersion, nil
}

// PodmanInfo is a wrapper around `podman info` command
func PodmanInfo() (map[string]interface{}, error) {
	args := []string{"info", "--format", "json"}
	output, err := CmdOutput(args...)
	if err != nil {
		return nil, err
	}

	var podmanInfo map[string]interface{}

	err = json.Unmarshal(output, &podmanInfo)
	if err != nil {
		return nil, err
	}

	return podmanInfo, nil
}

// PodmanInspect is a wrapper around 'podman inspect' command
//
// Parameter 'typearg' takes in values 'container' or 'image' that is passed to the --type flag
func PodmanInspect(typearg string, target string) (map[string]interface{}, error) {
	args := []string{"inspect", "--format", "json", "--type", typearg, target}
	output, err := CmdOutput(args...)
	if err != nil {
		return nil, err
	}

	var info []map[string]interface{}

	err = json.Unmarshal(output, &info)
	if err != nil {
		return nil, err
	}

	return info[0], nil
}

// GetContainers is a wrapper function around `podman ps --format json` command.
//
// Parameter args accepts an array of strings to be passed to the wrapped command (eg. ["-a", "--filter", "123"]).
//
// Returned value is a slice of dynamically unmarshalled json, so it needs to be treated properly.
//
// If a problem happens during execution, first argument is nil and second argument holds the error message.
func GetContainers(args ...string) ([]map[string]interface{}, error) {
	args = append([]string{"ps", "--format", "json"}, args...)
	output, err := CmdOutput(args...)
	if err != nil {
		return nil, err
	}

	var containers []map[string]interface{}

	err = json.Unmarshal(output, &containers)
	if err != nil {
		return nil, err
	}

	return containers, nil
}

// GetImages is a wrapper function around `podman images --format json` command.
//
// Parameter args accepts an array of strings to be passed to the wrapped command (eg. ["-a", "--filter", "123"]).
//
// Returned value is a slice of dynamically unmarshalled json, so it needs to be treated properly.
//
// If a problem happens during execution, first argument is nil and second argument holds the error message.
func GetImages(args ...string) ([]map[string]interface{}, error) {
	args = append([]string{"images", "--format", "json"}, args...)
	output, err := CmdOutput(args...)
	if err != nil {
		return nil, err
	}

	var images []map[string]interface{}

	err = json.Unmarshal(output, &images)
	if err != nil {
		return nil, err
	}

	return images, nil
}

// ImageExists checks using Podman if an image with given ID/name exists.
//
// Parameter image is a name or an id of an image.
func ImageExists(image string) (bool, error) {
	args := []string{"image", "exists", image}

	if err := CmdRun(args...); err != nil {
		return false, err
	}

	return true, nil
}

// ContainerExists checks using Podman if a container with given ID/name exists.
//
// Parameter container is a name or an id of a container.
func ContainerExists(container string) (bool, error) {
	args := []string{"container", "exists", container}

	if err := CmdRun(args...); err != nil {
		return false, err
	}

	return true, nil
}

// PullImage pulls an image
func PullImage(imageName string) error {
	args := []string{"--log-level", LogLevel, "pull", imageName}
	cmd := exec.Command("podman", args...)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	err := cmd.Run()
	if LogLevel == "debug" || LogLevel == "trace" {
		fmt.Fprint(os.Stderr, stderr.String())
	}

	if err != nil {
		if strings.Contains(stderr.String(), "invalid status code from registry 503 (Service Unavailable)") || strings.Contains(stderr.String(), "received unexpected HTTP status: 503 Service Temporarily Unavailable") {
			return ErrServiceUnavailable
		}
		if strings.Contains(stderr.String(), "read: connection refused") {
			return ErrConnectionRefused
		}
		if strings.Contains(stderr.String(), "manifest unknown: manifest unknown") {
			return ErrUnknownManifest
		}
		return err
	}

	return nil

}

// CmdOutput is a wrapper around Podman that returns the output of the invoked command.
//
// Parameter args accepts an array of strings to be passed to Podman.
//
// If no problem while executing a command occurs, then the output of the command is returned in the first value.
// If a problem occurs, then the error code is returned in the second value.
func CmdOutput(args ...string) ([]byte, error) {
	args = append([]string{"--log-level", LogLevel}, args...)
	cmd := exec.Command("podman", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if LogLevel == "debug" || LogLevel == "trace" {
		fmt.Fprint(os.Stderr, stderr.String())
	}

	if err != nil {
		return stderr.Bytes(), err
	}

	return stdout.Bytes(), nil
}

// CmdRun is a wrapper around Podman that does not return the output of the invoked command.
//
// Parameter args accepts an array of strings to be passed to Podman.
//
// If no problem while executing a command occurs, then the returned value is nil.
// If a problem occurs, then the error code is returned.
func CmdRun(args ...string) error {
	args = append([]string{"--log-level", LogLevel}, args...)
	cmd := exec.Command("podman", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if LogLevel == "debug" || LogLevel == "trace" {
		fmt.Fprint(os.Stderr, stderr.String())
	}

	if err != nil {
		return err
	}

	return nil
}

func CmdInto(args ...string) error {
	args = append([]string{"--log-level", LogLevel}, args...)
	cmd := exec.Command("podman", args...)

	cmd.Stdout = os.Stdout
	// Seems like there is no need to pipe the command stderr to the system one by default
	if LogLevel == "debug" || LogLevel == "trace" {
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = os.Stdin

	err := cmd.Run()

	if err != nil {
		return err
	}

	return nil
}

func SetLogLevel(logLevel string) error {
	if _, err := logrus.ParseLevel(logLevel); err != nil {
		return errors.New("failed to parse log-level")
	}

	LogLevel = logLevel
	return nil
}
