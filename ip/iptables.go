package ip

import (
	"github.com/action-io/goboxer/utils"
)

func forward(chain, port, dest string) error {
	return filter("-A", chain, port, dest)
}

func release(chain, port, dest string) error {
	return filter("-D", chain, port, dest)
}

func filter(action, chain, port, dest string) error {
	args := []string{
		"-t", "nat", // use nat table
		action, chain,
		"!", "-s", "192.168.1.0/16", // not forward traffic from internal
		"-p", "tcp", // forward tcp protocol
		"--dport", port, // from hostport
		"-j", "DNAT", // forward packet
		"--to", dest, // to ip:port
	}
	return utils.RunCommand("iptables", args...)
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
