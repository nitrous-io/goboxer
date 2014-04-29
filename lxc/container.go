package lxc

import (
	"fmt"
	"log"
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

// - Path: path to the container ,if this is empty, lxc will use the default
// which usually is /var/lib/lxc
// - Name: name of the container, typically mclovins-container-1-123456 with
// 123456 is the uniqueId
// - Namespace is typically mclovins-container-2
type Container struct {
	Path      string
	Name      string
	Namespace string
}

func (c *Container) Stop() error {
	logDir, err := createLogDir(c.Namespace)
	if err != nil {
		return err
	}
	logFile := logDir + "/" + c.Namespace + ".lxc-stop.log"
	// if the container is stopped, then we dont have to stop it again
	status, _ := Status(c.Path, c.Name)
	if status == STOPPED {
		log.Printf("lxc: Stop() - container %v already stopped\n", c.Name)
		return nil
	}
	run("lxc-stop", "--lxcpath", c.Path, "-n", c.Name, "-o", logFile,
		"-l", "INFO", "--timeout", fmt.Sprintf("%.0f", TIMEOUT.Seconds()))
	return c.waitForStatus(STOPPED, TIMEOUT)
}

func (c *Container) Start() error {
	logDir, err := createLogDir(c.Namespace)
	if err != nil {
		return err
	}
	logFile := logDir + "/" + c.Namespace + ".lxc-start.log"
	// if the container is running, then we dont have to start it again
	status, _ := c.Status()
	if status == RUNNING {
		log.Printf("lxc: Start() - container %v already started\n", c.Name)
		return nil
	}
	if err := run("lxc-start", "--lxcpath", c.Path, "-d",
		"-n", c.Name, "-o", logFile, "-l", "INFO"); err != nil {
		return err
	}
	// wait time is much shorter so some lxc process can perform works
	// before declaring the container status. Im looking at you lxc-start
	return c.waitForStatus(RUNNING, TIMEOUT)
}

// send SIGPWR to the init process of the container to poweroff the container
func (c *Container) Poweroff() error {
	pid, err := c.pid()
	// if we cant find pid, the container is probably not running
	if err != nil || pid == -1 {
		return nil
	}
	if err = syscall.Kill(pid, syscall.SIGPWR); err != nil {
		return err
	}
	return c.waitForStatus(STOPPED, TIMEOUT)
}

func (c *Container) Status() (string, error) {
	cmd := exec.Command("lxc-info", "--lxcpath", c.Path, "-s", "-n", c.Name)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("lxc: failed to get %s status: %v", c.Name, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) <= 1 {
		return "", fmt.Errorf("lxc: failed to get %s status: %v", c.Name, fields)
	}
	return strings.ToUpper(strings.TrimSpace(fields[1])), nil
}

func (c *Container) pid() (int, error) {
	cmd := exec.Command("lxc-info", "--lxcpath", c.Path, "-n", c.Name, "-p")
	output, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("lxc: failed to get %s pid: %v", c.Name, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) <= 1 {
		return -1, fmt.Errorf("lxc: failed to get %s pid: %v", c.Name, fields)
	}
	return strconv.Atoi(fields[1])
}

// wait until the container has the expected status or `TIMEOUT` which we
// return an error and let the caller decide what to do with it.
func (c *Container) waitForStatus(expected string, timeout time.Duration) error {
	cmd := exec.Command("lxc-wait", "--lxcpath", c.Path, "-n", c.Name,
		"-s", expected)
	return utils.Exec(cmd, timeout)
}
