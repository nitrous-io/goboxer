// wrapper around lxc operations
// TODO: when lxc 1.0 is released, we should be able to convert this package to
// native go-binding for lxc
package lxc

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/action-io/goboxer/utils"
)

const TIMEOUT = 30 * time.Second
const LOG_PREFIX = "/var/log/containers/"
const RUNNING = "RUNNING"
const STOPPED = "STOPPED"

func run(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func Stop(containerPath, namespace, containerName string) error {
	logDir, err := createLogDir(namespace)
	if err != nil {
		return err
	}
	logFile := logDir + "/" + namespace + ".lxc-stop.log"
	// if the container is stopped, then we dont have to stop it again
	status, _ := Status(containerPath, containerName)
	if strings.ToLower(status) == "stopped" {
		log.Printf("lxc: Stop() - container %v already stopped\n", containerName)
		return nil
	}
	run("lxc-stop", "--lxcpath", containerPath, "-n", containerName, "-o", logFile, "-l", "INFO")
	return waitForStatus(containerPath, containerName, STOPPED, TIMEOUT)
}

// this accepts a namespace and a containerName which will be used to log
// errors etc from the containers if happens
// namespace is typically mclovins-container-1
// containerName is typically mclovins-container-1-123456 with 123456 is the
// uniqueId
func Start(containerPath, namespace, containerName string) error {
	logDir, err := createLogDir(namespace)
	if err != nil {
		return err
	}
	logFile := logDir + "/" + namespace + ".lxc-start.log"
	// if the container is running, then we dont have to start it again
	status, _ := Status(containerPath, containerName)
	if strings.ToLower(status) == "running" {
		log.Printf("lxc: Start() - container %v already started\n", containerName)
		return nil
	}
	if err := run("lxc-start", "--lxcpath", containerPath, "-d",
		"-n", containerName, "-o", logFile, "-l", "INFO"); err != nil {
		return err
	}
	// wait time is much shorter so some lxc process can perform works
	// before declaring the container status. Im looking at you lxc-start
	return waitForStatus(containerPath, containerName, RUNNING, TIMEOUT)
}

// send SIGPWR to the init process of the container to poweroff the container
func Poweroff(containerPath, name string) error {
	pid, err := getContainerPid(containerPath, name)
	// if we cant find pid, the container is probably not running
	if err != nil || pid == -1 {
		return nil
	}
	if err = syscall.Kill(pid, syscall.SIGPWR); err != nil {
		return err
	}
	return waitForStatus(containerPath, name, STOPPED, TIMEOUT)
}

func Status(containerPath, name string) (string, error) {
	cmd := exec.Command("lxc-info", "--lxcpath", containerPath, "-s", "-n", name)
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

func CgroupGet(containerPath, name, subsystem string) (string, error) {
	cgroup := &Cgroup{Path: containerPath, Name: name}
	return cgroup.Get(subsystem)
}

func CgroupSet(containerPath, name, subsystem, value string) error {
	cgroup := &Cgroup{Path: containerPath, Name: name}
	return cgroup.Set(subsystem, value)
}

func getContainerPid(containerPath, name string) (int, error) {
	cmd := exec.Command("lxc-info", "--lxcpath", containerPath, "-n", name, "-p")
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
func waitForStatus(containerPath, name, expectedStatus string, timeout time.Duration) error {
	cmd := exec.Command("lxc-wait", "--lxcpath", containerPath, "-n", name, "-s", expectedStatus)
	return utils.Exec(cmd, timeout)
}

func createLogDir(namespace string) (string, error) {
	directory := LOG_PREFIX + namespace
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return directory, nil
}
