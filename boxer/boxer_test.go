package boxer

import (
	"github.com/action-io/goboxer/box"
	"github.com/action-io/goboxer/resource"
	"github.com/bmizerany/assert"
	"os/exec"
	"strings"
	"testing"
)

// returns a default boxer instance
func newTestBoxer() *Boxer {
	runtimes := map[string]string{
		"test": "test_container_template",
	}
	return &Boxer{Root: "/var/lib/lxc", Runtimes: runtimes}
}

func TestCreate(t *testing.T) {
	boxer := newTestBoxer()
	config := &box.Config{
		Id:          1,
		Name:        "mclovins-container",
		Runtime:     "test",
		Template:    "test_container_template",
		Memory:      "256M",
		DiskSpace:   "1G",
		PublicKeys:  []string{},
		SnapshotURL: "",
	}
	resource := &resource.Resource{
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

	box, _ := boxer.Create(config, resource)
	defer DestroyTestBox("mclovins-container-1", "192.168.254.2")
	assert.NotEqual(t, box, nil, "Failed to create a box")

	DoesExist(t, checkExists)
	DoesContain(t, checkContainsContent)
	DoesHavePermissions(t, checkPermissions)
}

func TestStop(t *testing.T) {
	CreateTestBox("1", "mclovins-container", "192.168.254.2", "10002")
	defer DestroyTestBox("mclovins-container-1", "192.268.254.2")

	boxer := newTestBoxer()
	config := &box.Config{
		Id:          1,
		Name:        "mclovins-container",
		Runtime:     "test",
		Template:    "test_container_template",
		Memory:      "256M",
		DiskSpace:   "1G",
		PublicKeys:  []string{},
		SnapshotURL: "",
	}
	resource := &resource.Resource{
		Ip:               "192.168.254.2",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10002",
	}
	box := &box.Box{Config: config, Resource: resource}
	err := boxer.Stop(box)
	assert.Equal(t, err, nil, "Stop should succeed")

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", box.ContainerName()).CombinedOutput()
	assert.Equal(t, strings.Contains(string(stdout), "STOPPED"), true,
		"box must be stopped")

	DoesNotExist(t, []string{
		"/var/lib/lxc/mclovins-container-1/rootfs",
	})

	DoesNotContain(t, map[string][]string{
		"/etc/projif":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/var/lib/lxc/mclovins-container-1_delta/rootfs/home/action"},
	})
}
