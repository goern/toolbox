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
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/containers/toolbox/pkg/podman"
	"github.com/containers/toolbox/pkg/utils"
	systemd "github.com/coreos/go-systemd/v22/dbus"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	createFlags struct {
		container string
		image     string
		release   int
	}

	createToolboxShMounts = []struct {
		containerPath string
		source        string
	}{
		{"/etc/profile.d/toolbox.sh", "/etc/profile.d/toolbox.sh"},
		{"/etc/profile.d/toolbox.sh", "/usr/share/profile.d/toolbox.sh"},
	}
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new toolbox container",
	RunE:  create,
}

func init() {
	flags := createCmd.Flags()

	flags.StringVarP(&createFlags.container,
		"container",
		"c",
		"",
		"Assign a different name to the toolbox container.")

	flags.StringVarP(&createFlags.image,
		"image",
		"i",
		"",
		"Change the name of the base image used to create the toolbox container.")

	flags.IntVarP(&createFlags.release,
		"release",
		"r",
		-1,
		"Create a toolbox container for a different operating system release than the host.")

	createCmd.SetHelpFunc(createHelp)
	rootCmd.AddCommand(createCmd)
}

func create(cmd *cobra.Command, args []string) error {
	if utils.IsInsideContainer() {
		if !utils.IsInsideToolboxContainer() {
			return errors.New("this is not a toolbox container")
		}

		if _, err := utils.ForwardToHost(); err != nil {
			return err
		}

		return nil
	}

	var container string
	var containerArg string

	if len(args) != 0 {
		container = args[0]
		containerArg = "CONTAINER"
	} else if createFlags.container != "" {
		container = createFlags.container
		containerArg = "--container"
	}

	if container != "" {
		if _, err := utils.IsContainerNameValid(container); err != nil {
			var builder strings.Builder
			fmt.Fprintf(&builder, "invalid argument for '%s'\n", containerArg)
			fmt.Fprintf(&builder, "Container names must match '%s'\n", utils.ContainerNameRegexp)
			fmt.Fprintf(&builder, "Run 'toolbox --help' for usage.")

			errMsg := builder.String()
			return errors.New(errMsg)
		}
	}

	var release string
	if createFlags.release > 0 {
		release = fmt.Sprint(createFlags.release)
	}

	container, image, release, err := utils.ResolveContainerAndImageNames(container,
		createFlags.image,
		release)
	if err != nil {
		return err
	}

	if err := createContainer(container, image, release); err != nil {
		return err
	}

	return nil
}

func createContainer(container, image, release string) error {
	if container == "" {
		panic("container not specified")
	}

	if image == "" {
		panic("image not specified")
	}

	if release == "" {
		panic("release not specified")
	}

	enterCommand := getEnterCommand(container, release)

	logrus.Debugf("Checking if container %s already exists", container)

	if _, err := podman.ContainerExists(container); err != nil {
		var builder strings.Builder
		fmt.Fprintf(&builder, "container %s already exists\n", container)
		fmt.Fprintf(&builder, "Enter with: %s\n", enterCommand)
		fmt.Fprintf(&builder, "Run 'toolbox --help' for usage.")

		errMsg := builder.String()
		return errors.New(errMsg)
	}

	if _, err := pullImage(image, release); err != nil {
		return nil
	}

	imageFull, err := getFullyQualifiedImageName(image)
	if err != nil {
		return err
	}

	toolboxPath := os.Getenv("TOOLBOX_PATH")
	toolboxPathEnvArg := "TOOLBOX_PATH=" + toolboxPath
	toolboxPathMountArg := toolboxPath + ":/usr/bin/toolbox:ro"

	logrus.Debug("Looking for group for sudo")

	sudoGroup, err := utils.GetGroupForSudo()
	if err != nil {
		return err
	}

	logrus.Debugf("Group for sudo is %s", sudoGroup)
	logrus.Debug("Checking if 'podman create' supports '--ulimit host'")

	var ulimitHost []string

	if podman.CheckVersion("1.5.0") {
		logrus.Debug("'podman create' supports '--ulimit host'")
		ulimitHost = []string{"--ulimit", "host"}
	}

	dbusSystemSocket, err := getDBusSystemSocket()
	if err != nil {
		return err
	}

	dbusSystemSocketMountArg := dbusSystemSocket + ":" + dbusSystemSocket

	flatpakHelperMonitorPath, err := utils.CallFlatpakSessionHelper()
	if err != nil {
		return err
	}

	flatpakHelperMonitorMountArg := flatpakHelperMonitorPath + ":/run/host/monitor"

	homeDirEvaled, err := filepath.EvalSymlinks(currentUser.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to canonicalize %s", currentUser.HomeDir)
	}

	logrus.Debugf("%s canonicalized to %s", currentUser.HomeDir, homeDirEvaled)
	homeDirMountArg := homeDirEvaled + ":" + homeDirEvaled + ":rslave"

	usrMountFlags := "ro"
	isUsrReadWrite, err := isUsrReadWrite()
	if err != nil {
		return err
	}
	if isUsrReadWrite {
		usrMountFlags = "rw"
	}

	usrMountArg := "/usr:/run/host/usr:" + usrMountFlags + ",rslave"

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	xdgRuntimeDirMountArg := xdgRuntimeDir + ":" + xdgRuntimeDir

	var kcmSocketMount []string

	kcmSocket, err := getKCMSocket()
	if err != nil {
		logrus.Debug(err)
	}
	if kcmSocket != "" {
		kcmSocketMountArg := kcmSocket + ":" + kcmSocket
		kcmSocketMount = []string{"--volume", kcmSocketMountArg}
	}

	logrus.Debug("Looking for toolbox.sh")

	var toolboxShMount []string

	for _, mount := range createToolboxShMounts {
		if utils.PathExists(mount.source) {
			logrus.Debugf("Found %s", mount.source)

			toolboxShMountArg := mount.source + ":" + mount.containerPath + ":ro"
			toolboxShMount = []string{"--volume", toolboxShMountArg}
			break
		}
	}

	var mediaLink []string
	var mediaMount []string

	if utils.PathExists("/media") {
		logrus.Debug("Checking if /media is a symbolic link to /run/media")

		mediaPath, _ := filepath.EvalSymlinks("/media")
		if mediaPath == "run/media" {
			logrus.Debug("/media is a symbolic link to /run/media")
			mediaLink = []string{"--media-link"}
		} else {
			mediaMount = []string{"--volume", "/media:/media:rslave"}
		}
	}

	logrus.Debug("Checking if /mnt is a symbolic link to /var/mnt")

	var mntLink []string
	var mntMount []string

	mntPath, _ := filepath.EvalSymlinks("/mnt")
	if mntPath == "var/mnt" {
		logrus.Debug("/mnt is a symbolic link to /var/mnt")
		mntLink = []string{"--mnt-link"}
	} else {
		mntMount = []string{"--volume", "/mnt:/mnt:rslave"}
	}

	var runMediaMount []string

	if utils.PathExists("/run/media") {
		runMediaMount = []string{"--volume", "/run/media:/run/media:rslave"}
	}

	logrus.Debug("Checking if /home is a symbolic link to /var/home")

	var slashHomeLink []string

	slashHomeEvaled, _ := filepath.EvalSymlinks("/home")
	if slashHomeEvaled == "/var/home" {
		logrus.Debug("/home is a symbolic link to /var/home")
		slashHomeLink = []string{"--home-link"}
	}

	entryPoint := []string{
		"toolbox", "--log-level", "debug",
		"init-container",
		"--home", currentUser.HomeDir,
	}

	entryPoint = append(entryPoint, slashHomeLink...)
	entryPoint = append(entryPoint, mediaLink...)
	entryPoint = append(entryPoint, mntLink...)

	entryPoint = append(entryPoint, []string{
		"--monitor-host",
		"--shell", currentUserShell,
		"--uid", currentUser.Uid,
		"--user", currentUser.Username,
	}...)

	createArgs := []string{
		"create",
		"--dns", "none",
		"--env", toolboxPathEnvArg,
		"--group-add", sudoGroup,
		"--hostname", "toolbox",
		"--ipc", "host",
		"--label", "com.github.containers.toolbox=true",
		"--label", "com.github.debarshiray.toolbox=true",
		"--name", container,
		"--network", "host",
		"--no-hosts",
		"--pid", "host",
		"--privileged",
		"--security-opt", "label=disable",
	}

	createArgs = append(createArgs, ulimitHost...)

	createArgs = append(createArgs, []string{
		"--userns=keep-id",
		"--user", "root:root",
		"--volume", "/etc:/run/host/etc",
		"--volume", "/dev:/dev:rslave",
		"--volume", "/run:/run/host/run:rslave",
		"--volume", "/tmp:/run/host/tmp:rslave",
		"--volume", "/var:/run/host/var:rslave",
		"--volume", dbusSystemSocketMountArg,
		"--volume", flatpakHelperMonitorMountArg,
		"--volume", homeDirMountArg,
		"--volume", toolboxPathMountArg,
		"--volume", usrMountArg,
		"--volume", xdgRuntimeDirMountArg,
	}...)

	createArgs = append(createArgs, kcmSocketMount...)
	createArgs = append(createArgs, mediaMount...)
	createArgs = append(createArgs, mntMount...)
	createArgs = append(createArgs, runMediaMount...)
	createArgs = append(createArgs, toolboxShMount...)

	createArgs = append(createArgs, []string{
		imageFull,
	}...)

	createArgs = append(createArgs, entryPoint...)

	logrus.Debugf("Creating container %s", container)
	logrus.Debug(createArgs)

	s := spinner.New(spinner.CharSets[9], 500*time.Millisecond)
	s.Prefix = fmt.Sprintf("Creating container %s: ", container)
	s.Writer = os.Stdout
	s.Start()
	defer s.Stop()

	if _, err := podman.CmdOutput(createArgs...); err != nil {
		return fmt.Errorf("failed to create container %s", container)
	}

	fmt.Printf("Created container: %s\n", container)
	fmt.Printf("Enter with: %s\n", enterCommand)

	return nil
}

func createHelp(cmd *cobra.Command, args []string) {
	if err := utils.ShowManual("toolbox-create"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
}

func getDBusSystemSocket() (string, error) {
	logrus.Debug("Resolving path to the D-Bus system socket")

	address := os.Getenv("DBUS_SYSTEM_BUS_ADDRESS")
	if address == "" {
		address = "unix:path=/var/run/dbus/system_bus_socket"
	}

	addressSplit := strings.Split(address, "=")
	if len(addressSplit) != 2 {
		return "", errors.New("failed to get the path to the D-Bus system socket")
	}

	path := addressSplit[1]
	pathEvaled, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("failed to resolve the path to the D-Bus system socket")
	}

	return pathEvaled, nil
}

func getEnterCommand(container, release string) string {
	var enterCommand string

	switch container {
	case utils.ContainerNameDefault:
		enterCommand = fmt.Sprintf("%s enter", executableBase)
	case utils.ContainerNamePrefixDefault:
		enterCommand = fmt.Sprintf("%s enter --release %s", executableBase, release)
	default:
		enterCommand = fmt.Sprintf("%s enter --container %s", executableBase, container)
	}

	return enterCommand
}

func getFullyQualifiedImageName(image string) (string, error) {
	var imageFull string

	if utils.ImageReferenceHasDomain(image) {
		imageFull = image
	} else {
		info, err := podman.PodmanInspect("image", image)
		if err != nil {
			return "", fmt.Errorf("failed to inspect image %s", image)
		}

		if info["RepoTags"] == nil {
			return "", fmt.Errorf("missing RepoTag for image %s", image)
		}

		repoTags := info["RepoTags"].([]string)
		if len(repoTags) == 0 {
			return "", fmt.Errorf("empty RepoTag for image %s", image)
		}

		imageFull = repoTags[0]
	}

	return imageFull, nil
}

func getKCMSocket() (string, error) {
	logrus.Debug("Resolving path to the KCM socket")

	connection, err := systemd.NewSystemConnection()
	if err != nil {
		return "", errors.New("failed to connect to the D-Bus system instance")
	}

	defer connection.Close()

	properties, err := connection.GetAllProperties("sssd-kcm.socket")
	if err != nil {
		return "", errors.New("failed to get the properties of sssd-kcm.socket")
	}

	value := properties["Listen"]
	if value == nil {
		return "", errors.New("failed to find the Listen property of sssd-kcm.socket")
	}

	sockets := value.([][]interface{})
	for _, socket := range sockets {
		if socket[0] == "Stream" {
			path := socket[1].(string)
			if !strings.HasPrefix(path, "/") {
				continue
			}

			pathEvaled, err := filepath.EvalSymlinks(path)
			if err != nil {
				continue
			}

			return pathEvaled, nil
		}
	}

	return "", errors.New("failed to find a SOCK_STREAM socket for sssd-kcm.socket")
}

func isUsrReadWrite() (bool, error) {
	logrus.Debug("Checking if /usr is mounted read-only or read-write")

	mountPoint, err := utils.GetMountPoint("/usr")
	if err != nil {
		return false, errors.New("failed to get the mount-point of /usr")
	}

	logrus.Debugf("Mount-point of /usr is %s", mountPoint)

	mountFlags, err := utils.GetMountOptions(mountPoint)
	if err != nil {
		return false, fmt.Errorf("failed to get the mount options of %s", mountPoint)
	}

	logrus.Debugf("Mount flags of /usr on the host are %s", mountFlags)

	if !strings.Contains(mountFlags, "ro") {
		return true, nil
	}

	return false, nil
}

func pullImage(image, release string) (bool, error) {
	if _, err := utils.ImageReferenceCanBeID(image); err == nil {
		logrus.Debugf("Looking for image %s", image)

		if _, err := podman.ImageExists(image); err == nil {
			return true, nil
		}
	}

	hasDomain := utils.ImageReferenceHasDomain(image)

	if !hasDomain {
		imageLocal := "localhost/" + image
		logrus.Debugf("Looking for image %s", imageLocal)

		if _, err := podman.ImageExists(imageLocal); err == nil {
			return true, nil
		}
	}

	var imageFull string

	if hasDomain {
		imageFull = image
	} else {
		imageFull = fmt.Sprintf("registry.fedoraproject.org/f%s/%s", release, image)
	}

	logrus.Debugf("Looking for image %s", imageFull)

	if _, err := podman.ImageExists(imageFull); err == nil {
		return true, nil
	}

	domain := utils.ImageReferenceGetDomain(imageFull)
	if domain == "" {
		panicMsg := fmt.Sprintf("failed to get domain from %s", imageFull)
		panic(panicMsg)
	}

	promptForDownload := true
	var shouldPullImage bool

	if rootFlags.assumeYes || domain == "localhost" {
		promptForDownload = false
		shouldPullImage = true
	}

	if promptForDownload {
		fmt.Println("Image required to create toolbox container.")

		prompt := fmt.Sprintf("Download %s (500MB)? [y/N]:", imageFull)
		shouldPullImage = utils.AskForConfirmation(prompt)
	}

	if !shouldPullImage {
		return false, errors.New("cancelled by user")
	}

	logrus.Debugf("Pulling image %s", imageFull)

	s := spinner.New(spinner.CharSets[9], 500*time.Millisecond)
	s.Prefix = fmt.Sprintf("Pulling %s: ", imageFull)
	s.Writer = os.Stdout
	s.Start()
	defer s.Stop()

	if err := podman.PullImage(imageFull); err != nil {
		return false, err
	}

	return true, nil
}
