package goboxer

import (
	. "launchpad.net/gocheck"
	"os/exec"
	"strings"
)

type BoxerSuite struct{}

var _ = Suite(&BoxerSuite{})

// returns a default boxer instance
func newTestBoxer() *Boxer {
	runtimes := map[string]string{
		"test": "test_container_template",
	}
	return &Boxer{Root: "/var/lib/lxc", Runtimes: runtimes}
}

func (s *BoxerSuite) TestStart(c *C) {
	CreateTestBox("1", "mclovins-container", "192.168.254.2", "10002",
		"dont-start")
	defer DestroyTestBox("mclovins-container-1", "192.268.254.2")

	boxer := newTestBoxer()
	box := &Box{
		Id:               "1",
		Name:             "mclovins-container",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10002",
	}
	err := boxer.Start(box)

	c.Assert(err.Success(), Equals, true)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", box.ContainerName()).CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "RUNNING"), Equals, true)

	DoesExist(c, []string{
		"/var/lib/lxc/mclovins-container-1/rootfs",
	})

	DoesContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/var/lib/lxc/mclovins-container-1_delta/rootfs/home/action"},
	})
}

func (s *BoxerSuite) TestDestroy(c *C) {
	CreateTestBox("1", "mclovins-container", "192.168.254.2", "10002")
	defer DestroyTestBox("mclovins-container-1", "192.268.254.2")

	box := &Box{
		Id:               "1",
		Name:             "mclovins-container",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10002",
	}
	boxer := newTestBoxer()
	boxer.Stop(box)
	boxer.Destroy(box)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", box.ContainerName()).CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "STOPPED"), Equals, true)

	DoesNotExist(c, []string{
		"/var/lib/lxc/mclovins-container-1",
		"/var/lib/lxc/mclovins-container-1_delta",
	})

	DoesNotContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/var/lib/lxc/mclovins-container-1_delta/rootfs/home/action"},
	})

	output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
	c.Assert(strings.Contains(string(output), "192.168.254.254:22"), Equals, false)
}

func (s *BoxerSuite) TestRestart(c *C) {
	CreateTestBox("1", "mclovins-container", "192.168.254.2", "10002")
	defer DestroyTestBox("mclovins-container-1", "192.268.254.2")

	box := &Box{
		Id:               "1",
		Name:             "mclovins-container",
		Runtime:          "test",
		Template:         "test_container_template",
		Memory:           "256M",
		DiskSpace:        "1G",
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10002",
	}
	boxer := newTestBoxer()
	err := boxer.Restart(box)
	c.Assert(err.Success(), Equals, true)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", box.ContainerName()).CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "RUNNING"), Equals, true)

	DoesExist(c, []string{
		"/var/lib/lxc/mclovins-container-1",
		"/var/lib/lxc/mclovins-container-1_delta",
	})

	DoesContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/var/lib/lxc/mclovins-container-1_delta/rootfs/home/action"},
	})
}
