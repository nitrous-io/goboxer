package goboxer

import (
	"testing"
)

// returns a default boxer instance
func newTestBoxer() *Boxer {
	runtimes := map[string]string{
		"test": "test_container_template",
	}
	return &Boxer{root: "/var/lib/lxc", runtimes: runtimes}
}

func TestCreate(t *testing.T) {
	var box *Box
	var err error
	boxer := newTestBoxer()
	config := &Config{
		Id:          1,
		Name:        "mclovins-container",
		Runtime:     "test",
		Template:    "test_container_template",
		Memory:      "256M",
		DiskSpace:   "1G",
		PublicKeys:  []string{},
		SnapshotURL: "",
	}
	resource := &Resource{
		Ip:               "192.168.254.253",
		Gateway:          "192.168.254.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10000",
	}
	box, err = boxer.Create(config, resource)
	if err != nil || box == nil {
		t.Errorf("Failed to create a box")
	}
	defer DestroyTestBox("mclovins-container-1", "192.168.254.253")

	// assert the box
	containerPath := "/var/lib/lxc/mclovins-container-1"
	var res bool
	res, err = Exists(containerPath)
	if !res || err != nil {
		t.Errorf("Failed to create the container")
	}
	res, err = Exists(containerPath + "_delta")
	if !res || err != nil {
		t.Errorf("Failed to create the container delta")
	}

	res, err = FileContainsString("/etc/projid", "mclovins-container-1:1")
	if !res || err != nil {
		t.Errorf("/etc/projid does not contain correct container info", err)
	}

	res, err = FileContainsString("/etc/projects", "1:"+containerPath)
	if !res || err != nil {
		t.Errorf("/etc/projects does not have correct mapping", err)
	}

	res, err = Exists(containerPath + "/rootfs/home/action/.ssh/id_rsa.pub")
	if !res || err != nil {
		t.Errorf("Failed to create the public key")
	}
	res, err = Exists(containerPath + "/rootfs/home/action/.ssh/id_rsa")
	if !res || err != nil {
		t.Errorf("Failed to create the private key")
	}
	res, err = Exists(containerPath + "/rootfs/home/action/workspace")
	if !res || err != nil {
		t.Errorf("Failed to create the workspace")
	}
	res, err = FileContainsString(containerPath+"/rootfs/etc/passwd", "action:x:15001:15001")
	if !res || err != nil {
		t.Errorf("containers' /etc/passwd does not contain correct uid:gid")
	}

	configFile := containerPath + "/config"
	res, err = FileContainsString(configFile, "lxc.cgroup.memory.limit_in_bytes = 256M")
	if !res || err != nil {
		t.Errorf("container does not have correct memory limit")
	}
	res, err = FileContainsString(configFile, "lxc.network.hwaddr = 02:00")
	if !res || err != nil {
		t.Errorf("container does not have correct hardware address")
	}
	res, err = FileContainsString(configFile, "lxc.cgroup.memory.memsw.limit_in_bytes = 768M")
	if !res || err != nil {
		t.Errorf("container does not have correct swap + memory limit")
	}

	res, err = FileContainsString(configFile, "lxc.cgroup.memory.memsw.limit_in_bytes = 768M")
	if !res || err != nil {
		t.Errorf("container does not have correct swap + memory limit")
	}

	res, err = FileContainsString(containerPath+"/rootfs/etc/action/box_id", "1")
	if !res || err != nil {
		t.Errorf("incorrect box_id.Expect %s but got %s", "1", err)
	}

	var uid, gid int

	uid, err = GetUid(containerPath + "/rootfs/home/action")
	if uid != 15001 {
		t.Errorf("wrong uid for home. Expected %d, got %d", 15001, uid)
	}
	gid, err = GetGid(containerPath + "/rootfs/home/action")
	if uid != 15001 {
		t.Errorf("wrong gid for home. Expected %d, got %d", 15001, gid)
	}

	uid, err = GetUid(containerPath + "/rootfs/home/action/workspace")
	if uid != 15001 {
		t.Errorf("wrong uid for workspace. Expected %d, got %d", 15001, uid)
	}
	gid, err = GetGid(containerPath + "/rootfs/home/action/workspace")
	if uid != 15001 {
		t.Errorf("wrong gid for workspace. Expected %d, got %d", 15001, gid)
	}
}
