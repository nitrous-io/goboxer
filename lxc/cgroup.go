package lxc

import (
	"fmt"
	"log"
	"os/exec"
)

type Cgroup struct {
	Path string
	Name string
}

func (c *Cgroup) Get(subsystem string) (string, error) {
	args := append(c.args(), subsystem)
	output, err := exec.Command("lxc-cgroup", args...).Output()
	return string(output), err
}

func (c *Cgroup) Set(subsystem, value string) error {
	log.Printf("lxc: CgroupSet(%s, %s, %s)", c.Name, subsystem, value)
	status, err := Status(c.Path, c.Name)
	if err != nil {
		return err
	}
	if status != RUNNING {
		return fmt.Errorf("Tried to CgroupSet on non-running container %s",
			c.Name)
	}
	args := append(c.args(), subsystem, value)
	return exec.Command("lxc-cgroup", args...).Run()
}

// if Path is not empty, then we set --lxcpath values so lxc-cgroup can
// understand how to find it
func (c *Cgroup) args() []string {
	args := []string{}
	if len(c.Path) > 0 {
		args = append(args, "--lxcpath", c.Path)
	}
	return append(args, "-n", c.Name)
}
