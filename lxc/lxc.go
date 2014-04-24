// wrapper around lxc operations
// TODO: when lxc 1.0 is released, we should be able to convert this package to
// native go-binding for lxc
package lxc

import (
	"os"
	"os/exec"
)

func run(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func Stop(containerPath, namespace, containerName string) error {
	c := &Container{Path: containerPath, Namespace: namespace, Name: containerName}
	return c.Stop()
}

func Start(containerPath, namespace, containerName string) error {
	c := &Container{Path: containerPath, Namespace: namespace, Name: containerName}
	return c.Start()
}

// send SIGPWR to the init process of the container to poweroff the container
func Poweroff(containerPath, name string) error {
	c := &Container{Path: containerPath, Name: name}
	return c.Poweroff()
}

func Status(containerPath, name string) (string, error) {
	c := &Container{Path: containerPath, Name: name}
	return c.Status()
}

func CgroupGet(containerPath, name, subsystem string) (string, error) {
	cgroup := &Cgroup{Path: containerPath, Name: name}
	return cgroup.Get(subsystem)
}

func CgroupSet(containerPath, name, subsystem, value string) error {
	cgroup := &Cgroup{Path: containerPath, Name: name}
	return cgroup.Set(subsystem, value)
}

func createLogDir(namespace string) (string, error) {
	directory := LOG_PREFIX + namespace
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return directory, nil
}
