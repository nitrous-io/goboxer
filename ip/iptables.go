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

func natRule(ruleType, chain, port, dest string) []string {
	return []string{
		"-t", "nat", // use nat table
		ruleType, chain, // append to chain
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", port, // from hostport
		"-j", "DNAT", // forward packet
		"--to", dest, // to ip:port
	}
}

func mangleRule(ruleType, chain, port, mark string) []string {
	return []string{
		ruleType, chain,
		"-t", "mangle",
		"-p", "tcp",
		"--sport", port,
		"-j", "MARK",
		"--set-mark", mark,
	}
}

func forward(chain, port, dest string) error {
	args := []string{
		"-t", "nat", // use nat table
		"-A", chain, // append to chain
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", port, // from hostport
		"-j", "DNAT", // forward packet
		"--to", dest, // to ip:port
	}
	return runCommand("iptables", args...)
}

func release(chain, port, dest string) error {
	args := []string{
		"-t", "nat", // use nat table
		"-D", chain, // delete from chain
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", port, // from hostport
		"-j", "DNAT", // forward packet
		"--to", dest, // to ip:port
	}
	return runCommand("iptables", args...)
}

func Open(hostPort, containerIp, containerPort string) error {
	return forward("PREROUTING", hostPort, containerIp+":"+containerPort)
}

func OpenInVagrant(hostPort, containerIp, containerPort string) error {
	if err := Open(hostPort, containerIp, containerPort); err != nil {
		return err
	}
	return forward("OUTPUT", hostPort, containerIp+":"+containerPort)
}

func Close(hostPort, containerIp, containerPort string) error {
	return release("PREROUTING", hostPort, containerIp+":"+containerPort)
}

func CloseInVagrant(hostPort, containerIp, containerPort string) error {
	if err := Close(hostPort, containerIp, containerPort); err != nil {
		return err
	}
	return release("OUTPUT", hostPort, containerIp+":"+containerPort)
}

func Mark(port, mark string) error {
	rule := mangleRule("-A", "OUTPUT", port, mark)
	return runCommand("iptables", rule...)
}

func Unmark(port, mark string) error {
	rule := mangleRule("-D", "OUTPUT", port, mark)
	return runCommand("iptables", rule...)
}
