package ip

import (
	"github.com/action-io/goboxer/utils"
	"os/exec"
)

type ethRule struct {
	Source    string
	IsVagrant bool
}

func (r ethRule) args(ruleType string) []string {
	args := []string{
		"-t", "nat", // use nat table
		ruleType, "POSTROUTING", // append to chain
		"-j", "MASQUERADE",
	}
	if r.IsVagrant {
		args = append(args, "-s", r.Source, "!", "-d", r.Source)
	} else {
		args = append(args, "-o", "eth0", "-s", r.Source)
	}
	return args
}

func (r ethRule) create() error {
	if r.exists() {
		return nil
	}
	return utils.RunCommand("iptables", r.args("-A")...)
}

func (r ethRule) destroy() error {
	if r.exists() {
		return utils.RunCommand("iptables", r.args("-D")...)
	}
	return nil
}

func (r ethRule) exists() bool {
	cmd := exec.Command("iptables", r.args("-C")...)
	cmd.Run()
	return cmd.ProcessState.Success()
}
