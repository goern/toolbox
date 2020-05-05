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

package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/acobaugh/osrelease"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var (
	preservedEnvironmentVariables = []string{
		"COLORTERM",
		"DBUS_SESSION_BUS_ADDRESS",
		"DBUS_SYSTEM_BUS_ADDRESS",
		"DESKTOP_SESSION",
		"DISPLAY",
		"LANG",
		"SHELL",
		"SSH_AUTH_SOCK",
		"TERM",
		"TOOLBOX_PATH",
		"VTE_VERSION",
		"WAYLAND_DISPLAY",
		"XDG_CURRENT_DESKTOP",
		"XDG_DATA_DIRS",
		"XDG_MENU_PREFIX",
		"XDG_RUNTIME_DIR",
		"XDG_SEAT",
		"XDG_SESSION_DESKTOP",
		"XDG_SESSION_ID",
		"XDG_SESSION_TYPE",
		"XDG_VTNR",
	}
)

func ForwardToHost() (int, error) {
	envOptions := GetEnvOptionsForPreservedVariables()
	toolboxPath := os.Getenv("TOOLBOX_PATH")

	var flatpakSpawnArgs []string

	flatpakSpawnArgs = append(flatpakSpawnArgs, envOptions...)

	flatpakSpawnArgs = append(flatpakSpawnArgs, []string{
		"--host",
		toolboxPath,
	}...)

	flatpakSpawnArgs = append(flatpakSpawnArgs, os.Args[1:]...)

	logrus.Debug("Forwarding to host:")
	logrus.Debug(flatpakSpawnArgs)

	cmd := exec.Command("flatpak-spawn", flatpakSpawnArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	if logLevel := logrus.GetLevel(); logLevel >= logrus.DebugLevel {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 1, errors.New("flatpak-spawn(1) not found")
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode := exitErr.ExitCode()
			return exitCode, nil
		}
	}

	return 0, nil
}

// GetCgroupsVersion returns the cgroups version of the host
//
// Based on the IsCgroup2UnifiedMode function in:
// https://github.com/containers/libpod/tree/master/pkg/cgroups
func GetCgroupsVersion() (int, error) {
	var st syscall.Statfs_t

	if err := syscall.Statfs("/sys/fs/cgroup", &st); err != nil {
		return -1, err
	}

	version := 1
	if st.Type == unix.CGROUP2_SUPER_MAGIC {
		version = 2
	}

	return version, nil
}

func GetEnvOptionsForPreservedVariables() []string {
	var envOptions []string

	for _, variable := range preservedEnvironmentVariables {
		value, found := os.LookupEnv(variable)
		if !found {
			continue
		}

		envOptions = append(envOptions, fmt.Sprintf("--env=%s=%s", variable, value))
	}

	return envOptions
}

// GetHostID returns the ID from the os-release files
//
// Examples:
// - host is Fedora, returned string is 'fedora'
func GetHostID() (string, error) {
	osRelease, err := osrelease.Read()
	if err != nil {
		return "", err
	}

	return osRelease["ID"], nil
}

// GetHostVariantID returns the VARIANT_ID from the os-release files
//
// Examples:
// - host is Fedora Workstation, returned string is 'workstation'
func GetHostVariantID() (string, error) {
	osRelease, err := osrelease.Read()
	if err != nil {
		return "", err
	}

	return osRelease["VARIANT_ID"], nil
}

// GetHostVersionID returns the VERSION_ID from the os-release files
//
// Examples:
// - host is Fedora 32, returned string is '32'
func GetHostVersionID() (string, error) {
	osRelease, err := osrelease.Read()
	if err != nil {
		return "", err
	}

	return osRelease["VERSION_ID"], nil
}

// PathExists wraps around os.Stat providing a nice interface for checking an existence of a path.
func PathExists(path string) bool {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return true
	}

	return false
}

func IsInsideContainer() bool {
	if PathExists("/run/.containerenv") {
		return true
	}

	return false
}

func IsInsideToolboxContainer() bool {
	if PathExists("/run/.toolboxenv") {
		return true
	}

	return false
}

func ShowManual(manual string) error {
	manBinary, err := exec.LookPath("man")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("man(1) not found")
		}

		return errors.New("failed to lookup man(1)")
	}

	manualArgs := []string{"man", manual}
	env := os.Environ()

	if err := syscall.Exec(manBinary, manualArgs, env); err != nil {
		return errors.New("failed to invoke man(1)")
	}

	return nil
}
