package ip

import (
	"github.com/action-io/goboxer/utils"
	"os/exec"
)

type natRule struct {
	Chain string
	Port  string
	Dest  string
}

func (n natRule) args(ruleType string) []string {
	return []string{
		"-t", "nat", // use nat table
		ruleType, n.Chain, // append to chain
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", n.Port, // from hostport
		"-j", "DNAT", // forward packet
		"--to", n.Dest, // to ip:port
	}

}

func (n natRule) create() error {
	if n.exists() {
		return nil
	}
	return utils.RunCommand("iptables", n.args("-A")...)
}

func (n natRule) destroy() error {
	if n.exists() {
		return utils.RunCommand("iptables", n.args("-D")...)
	}
	return nil
}

func (n natRule) exists() bool {
	cmd := exec.Command("iptables", n.args("-C")...)
	cmd.Run()
	return cmd.ProcessState.Success()
}
