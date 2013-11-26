// wrapper around lxc operations
// TODO: when lxc 1.0 is released, we should be able to convert this package to
// native go-binding for lxc
package lxc

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

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

func Stop(name string) error {
	// FIXME: once lxc-shutdown is upgraded to a new version, restore the error
	// checking here
	run("lxc-shutdown", "-n", name, "-t", "30")
	waitLxc(name, "STOPPED")
	state, err := Status(name)
	if err != nil || state != "STOPPED" {
		return fmt.Errorf("lxc: stop failed %v %v %v", name, state, err)
	}
	return nil
}

func Status(name string) (string, error) {
	cmd := exec.Command("lxc-info", "-n", name)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// found the current state of the container
		// return immediately
		if fields[0] == "state:" {
			return fields[1], nil
		}
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("lxc: Status failed %v", name)
}
