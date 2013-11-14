package ip

import (
	"fmt"
	"os/exec"
)

// FIXME: this is copied from the main package. should consider moving it to
// utils
func runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	if stdout, err := command.CombinedOutput(); err != nil {
		fmt.Printf("Command %v failed with output %s\n", command, string(stdout))
		return err
	}
	return nil
}

func Open(hostPort, containerIp, containerPort string) error {
	args := []string{
		"-t", "nat", // use nat table
		"-A", "PREROUTING", // append to PREROUTING chain
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", hostPort, // from hostport
		"-j", "DNAT", // forward packet
		"--to", containerIp + ":" + containerPort, // to ip:port
	}
	return runCommand("iptables", args...)
}

func Close(hostPort, containerIp, containerPort string) error {
	args := []string{
		"-t", "nat", // use nat table
		"-D", "PREROUTING", // delete from PREROUTING chain
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", hostPort, // from hostport
		"-j", "DNAT", // forward packet
		"--to", containerIp + ":" + containerPort, // to ip:port
	}
	return runCommand("iptables", args...)
}
