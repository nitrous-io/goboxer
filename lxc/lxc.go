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
)

const WAIT_TIME = 1 * time.Second
const TIMEOUT = 10 * time.Second
const LOG_PREFIX = "/var/log/containers/"

func run(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func output(cmd string, args ...string) ([]byte, error) {
	return exec.Command(cmd, args...).CombinedOutput()
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
	if err := run("lxc-start", "-d", "-n", containerName,
		"-o", logFile, "-l", "INFO"); err != nil {
		return err
	}
	return waitForStatus(containerName, "RUNNING")
}

// implementation of lxc-shutdown
//
// - first, we locate the init process of the container via lxc-info
// - then  we send SIGPWR to that process indicate that we want the container
// to shutdown /poweroff
// - then we wait until either the container status become stopped, or
// `TIMEOUT` is up which we execute lxc-stop immediately
func Stop(name string) error {
	pid, err := getContainerPid(name)
	// if we cant find pid, the container is probably not running
	if err != nil || pid == -1 {
		return nil
	}
	if err = syscall.Kill(pid, syscall.SIGPWR); err != nil {
		return err
	}
	// SIGPWR cant stop the container, do the hard-work here
	if waitForStatus(name, "STOPPED") != nil {
		return run("lxc-stop", "-n", name)
	}
	return nil
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

// wait until the container has the expected status or `TIMEOUT` which we
// return an error and let the caller decide what to do with it.
func waitForStatus(name, expectedStatus string) error {
	c := make(chan error, 1)
	go func() {
		// we dont want this to run forever, just until `TIMEOUT` is fine
		for now := time.Now(); time.Since(now) < TIMEOUT; {
			status, err := Status(name)
			if err != nil || status == expectedStatus {
				c <- nil
				return
			}
			time.Sleep(WAIT_TIME)
		}
	}()
	select {
	case _ = <-c:
		return nil
	case <-time.After(TIMEOUT):
		return fmt.Errorf("lxc: %s failed to have status %s", name,
			expectedStatus)
	}
	return nil
}
