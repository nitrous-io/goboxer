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
	"log" // REMOVEME
)

const TIMEOUT = 30 * time.Second
const LOG_PREFIX = "/var/log/containers/"
const RUNNING = "RUNNING"
const STOPPED = "STOPPED"

func run(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func output(cmd string, args ...string) ([]byte, error) {
	return exec.Command(cmd, args...).CombinedOutput()
}

func Stop(namespace, containerName string) error {
	logDir, err := createLogDir(namespace)
	if err != nil {
		return err
	}
	logFile := logDir + "/" + containerName + ".lxc-stop.log"
	// if the container is stopped, then we dont have to stop it again
	status, _ := Status(containerName)
	if strings.ToLower(status) == "stopped" {
		log.Printf("lxc: Stop() - container %v already stopped\n", containerName)
		return nil
	}
	run("lxc-stop", "-n", containerName, "-o", logFile, "-l", "INFO")
	return waitForStatus(containerName, STOPPED, TIMEOUT)
}

// this accepts a namespace and a containerName which will be used to log
// errors etc from the containers if happens
// namespace is typically mclovins-container-1
// containerName is typically mclovins-container-1-123456 with 123456 is the
// uniqueId
func Start(namespace, containerName string) error {
	logDir, err := createLogDir(namespace)
	if err != nil {
		return err
	}
	logFile := logDir + "/" + containerName + ".lxc-start.log"
	// if the container is running, then we dont have to start it again
	status, _ := Status(containerName)
	if strings.ToLower(status) == "running" {
		log.Printf("lxc: Start() - container %v already started\n", containerName)
		return nil
	}
	if err := run("lxc-start", "-d", "-n", containerName,
		"-o", logFile, "-l", "INFO"); err != nil {
		return err
	}
	// wait time is much shorter so some lxc process can perform works
	// before declaring the container status. Im looking at you lxc-start
	return waitForStatus(containerName, RUNNING, TIMEOUT)
}

// send SIGPWR to the init process of the container to poweroff the container
func Poweroff(name string) error {
	log.Printf("REMOVEME: Setting pid, err for %s", name)
	pid, err := getContainerPid(name)
	log.Printf("REMOVEME: pid: %s, err: %s", pid, err)
	// if we cant find pid, the container is probably not running
	if err != nil || pid == -1 {
		log.Printf("REMOVEME: err != nil || pid == -1, err: %s, pid: %s", err, pid)
		return nil
	}
	if err = syscall.Kill(pid, syscall.SIGPWR); err != nil {
		log.Printf("REMOVEME: err = syscall.Kill(pid, syscall.SIGPWR); err != nil, err: %s", err)
		return err
	}
	log.Printf("REMOVEME: waitForStatus(%s, STOPPED, TIMEOUT)", name)
	return waitForStatus(name, STOPPED, TIMEOUT)
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
func waitForStatus(name, expectedStatus string, timeout time.Duration) error {
	cmd := exec.Command("lxc-wait", "-n", name, "-s", expectedStatus)
	return utils.Exec(cmd, timeout)
}

func createLogDir(namespace string) (string, error) {
	directory := LOG_PREFIX + namespace
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return directory, nil
}
