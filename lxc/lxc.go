// wrapper around lxc operations
// TODO: when lxc 1.0 is released, we should be able to convert this package to
// native go-binding for lxc
package lxc

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/action-io/goboxer/utils"
)

// base timeout for all lxc commands. This is not the timeout that we want
// for container to reach their expected status
const TIMEOUT = 30 * time.Second
const LOG_PREFIX = "/var/log/containers/"
const RUNNING = "RUNNING"
const STOPPED = "STOPPED"

func run(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func Stop(name string) error {
	cmd := exec.Command("lxc-stop", "-n", name)
	return utils.Exec(cmd, TIMEOUT)
}

// this accepts a namespace and a containerName which will be used to log
// errors etc from the containers if happens
// namespace is typically mclovins-container-1
// containerName is typically mclovins-container-1-123456 with 123456 is the
// uniqueId
func Start(namespace, containerName string) error {
	logDir := LOG_PREFIX + namespace
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	logFile := logDir + "/" + containerName + ".log"
	// try to log as much as possible
	cmd := exec.Command("lxc-start", "-d", "-n", containerName,
		"-o", logFile, "-l", "INFO")
	return utils.Exec(cmd, TIMEOUT)
}

// send SIGPWR to the init process of the container to poweroff the container
func Poweroff(name string) error {
	pid, err := getContainerPid(name)
	// if we cant find pid, the container is probably not running
	if err != nil || pid == -1 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGPWR)
}

func Status(name string) (string, error) {
	cmd := exec.Command("lxc-info", "-s", "-n", name)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("lxc: failed to get %s status: %v", name, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) <= 1 {
		return "", fmt.Errorf("lxc: failed to get %s status: %v", name, fields)
	}
	return strings.TrimSpace(fields[1]), nil
}

// wait until the container has the expected status or `TIMEOUT` which we
// return an error and let the caller decide what to do with it.
func Wait(name, expectedStatus string, timeout time.Duration) error {
	cmd := exec.Command("lxc-wait", "-n", name, "-s", expectedStatus)
	return utils.Exec(cmd, timeout)
}

func getContainerPid(name string) (int, error) {
	cmd := exec.Command("lxc-info", "-n", name, "-p")
	output, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("lxc: failed to get %s pid: %v", name, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) <= 1 {
		return -1, fmt.Errorf("lxc: failed to get %s pid: %v", name, fields)
	}
	return strconv.Atoi(fields[1])
}
