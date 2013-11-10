// wrapper around lxc operations
// TODO: when lxc 1.0 is released, we should be able to convert this package to
// native go-binding for lxc
package lxc

import (
	"fmt"
	"os/exec"
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
	return waitLxc(name, "STOPPED")
}

func Status(name string) (string, error) {
	var state, status string
	info, err := output("lxc-info", "-n", name)
	if err != nil {
		return "", err
	}
	_, err = fmt.Sscanln(string(info), &state, &status)
	return status, err
}
