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

func (s *BoxerSuite) TestCreate(c *C) {
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
		Port:             "10000",
	}
	root := "/var/lib/lxc/mclovins-container-1"
	var checkExists = []string{
		root,
		root + "_delta",
		root + "/rootfs",
		root + "/rootfs/home/action/workspace",
		root + "/rootfs/home/action/.ssh",
		root + "/rootfs/home/action/.ssh/id_rsa",
		root + "/rootfs/home/action/.ssh/id_rsa.pub",
	}

	var checkContainsContent = map[string][]string{
		"/etc/projid":                      {"mclovins-container-1:1"},
		"/etc/projects":                    {"1:/var/lib/lxc/mclovins-container-1"},
		root + "/rootfs/etc/passwd":        {"action:x:15001:15001"},
		root + "/rootfs/etc/action/box_id": {"1"},
		root + "/rootfs/etc/hostname":      {"mclovins-container-1"},
		root + "/rootfs/etc/network/interfaces": {
			"address 192.168.254.2",
			"gateway 192.168.254.1",
		},
		root + "/config": {
			"lxc.cgroup.memory.limit_in_bytes = 256M",
			"lxc.cgroup.memory.memsw.limit_in_bytes = 768M",
			"lxc.network.hwaddr = 02:00",
		},
	}

	var checkPermissions = map[string][]int{
		root + "/rootfs/home/action":                 {15001, 15001},
		root + "/rootfs/home/action/workspace":       {15001, 15001},
		root + "/rootfs/home/action/.ssh":            {15001, 15001},
		root + "/rootfs/home/action/.ssh/id_rsa":     {15001, 15001},
		root + "/rootfs/home/action/.ssh/id_rsa.pub": {15001, 15001},
	}

	boxer.Create(box)
	defer DestroyTestBox("mclovins-container-1", "192.168.254.2")

	c.Assert(box, NotNil)
	DoesExist(c, checkExists)
	DoesContain(c, checkContainsContent)
	DoesHavePermissions(c, checkPermissions)
}

func (s *BoxerSuite) TestStop(c *C) {
	CreateTestBox("1", "mclovins-container", "192.168.254.2", "10002")
	defer DestroyTestBox("mclovins-container-1", "192.168.254.2")

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
	err := boxer.Stop(box)
	c.Assert(err.Success(), Equals, true, Commentf("failed to stop the box"))

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", box.ContainerName()).CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "STOPPED"), Equals, true)

	DoesNotExist(c, []string{
		"/var/lib/lxc/mclovins-container-1/rootfs",
	})

	DoesNotContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/var/lib/lxc/mclovins-container-1_delta/rootfs/home/action"},
	})
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
