// wrapper around lxc operations
// TODO: when lxc 1.0 is released, we should be able to convert this package to
// native go-binding for lxc
package lxc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const WAIT_TIME = 1 * time.Second
const TIMEOUT = 10 * time.Second

func run(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func output(cmd string, args ...string) ([]byte, error) {
	return exec.Command(cmd, args...).CombinedOutput()
}

func waitLxc(name, status string) error {
	return run("lxc-wait", "-n", name, "-s", status)
}

func Start(name string) error {
	if err := run("lxc-start", "-d", "-n", name); err != nil {
		return err
	}
	return waitLxc(name, "RUNNING")
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
	c := make(chan bool, 1)
	go func(name string) {
		for {
			status, err := Status(name)
			if err != nil || status == "STOPPED" {
				c <- true
				break
			}
			time.Sleep(WAIT_TIME)
		}
	}(name)
	select {
	case _ = <-c:
		return nil
	case <-time.After(TIMEOUT):
		run("lxc-stop", "-n", name)
		return waitLxc(name, "STOPPED")
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
