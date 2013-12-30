package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSetNetwork(t *testing.T) {
	defer exec.Command("iptables", "-F", "-t", "nat").Run()
	setNetwork(253, 254, false)
	output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
	if strings.Count(string(output), "192.168.253.0/24") != 1 {
		t.Error("failed to set network")
	}
	if strings.Count(string(output), "192.168.254.0/24") != 1 {
		t.Error("failed to set network")
	}
}

func TestSetPortForwarding(t *testing.T) {
	defer exec.Command("iptables", "-F", "-t", "nat").Run()
	setPortForwarding(254, 254, false)
	output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
	if strings.Count(string(output), "dpt:10001 to:192.168.254.253:22") != 1 {
		t.Error("failed to set network")
	}
}
