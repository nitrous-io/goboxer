package ip

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/action-io/goboxer/utils"
)

type locker struct {
	sync.Mutex
}

var lock = locker{}

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
	ruleExists, err := n.exists()
	if err != nil || ruleExists {
		return err
	}
	lock.Lock()
	defer lock.Unlock()
	return utils.RunCommand("iptables", n.args("-A")...)
}

func (n natRule) destroy() error {
	ruleExists, err := n.exists()
	if err != nil || !ruleExists {
		return err
	}
	lock.Lock()
	defer lock.Unlock()
	return utils.RunCommand("iptables", n.args("-D")...)
}

func (n natRule) exists() (bool, error) {
	lock.Lock()
	defer lock.Unlock()
	// if iptables -C fails to find a matching rule,
	// it exits with exit status 1
	// So we need to differentiate that error from others
	cmd := exec.Command("iptables", n.args("-C")...)
	output, err := cmd.CombinedOutput()

	if cmd.ProcessState == nil {
		return false, fmt.Errorf("Unexpected nil ProcessState, cmd: %v, err: %v", cmd, err)
	}

	switch {
	case cmd.ProcessState.Success():
		return true, nil
	case strings.Contains(string(output), "does a matching rule exist"), strings.Contains(string(output), "No chain/target/match by that name"):
		return false, nil
	default:
		return false, fmt.Errorf("Unexpected iptables error output: %s", output)
	}
}
