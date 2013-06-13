// Integration test for goboxer
//
// This does not test how goboxer and coach interact with each other, only test
// the behavior within goboxer itself. Therefore, we dont want to involve
// pubsub testing here
package goboxer

import (
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
	"os/exec"
	"strings"
)

type BoxerSuite struct {
	Redis        redis.Conn
	Boxer        *Boxer
	ResourcePool *ResourcePool
}

var _ = Suite(&BoxerSuite{})

func (s *BoxerSuite) insertResource(ip, gateway, networkInterface, port string) {
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	s.Redis.Do("RPUSH", "aio:available_resources", ip)
	s.Redis.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *BoxerSuite) deleteBox(id, name string) Message {
	message, _ := s.Boxer.Receive(Message{
		Type:  "aio:destroy",
		Id:    "testboxer",
		BoxId: id,
		Name:  name + "-" + id,
	})
	return message
}

func (s *BoxerSuite) SetUpTest(c *C) {
	s.Redis, _ = redis.Dial("tcp", ":65000")
	s.Redis.Do("Select", 13)
	s.Redis.Do("FLUSHDB")
	s.insertResource("192.168.254.2", "192.168.254.1", "br-lxc-254", "10002")

	s.ResourcePool = &ResourcePool{Conn: s.Redis}

	s.Boxer = &Boxer{
		Id:           "testboxer",
		Root:         "/var/lib/lxc",
		Disks:        []string{"/lxc/disk1"},
		SelectedDisk: "/lxc/disk1",
		Runtimes: map[string]string{
			"test": "test_container_template",
		},
		ResourcePool: s.ResourcePool,
		Handles: map[string]MessageHandler{
			"aio:locate":    LocateBoxHandler,
			"aio:create":    CreateBoxHandler,
			"aio:start":     StartBoxHandler,
			"aio:stop":      StopBoxHandler,
			"aio:restart":   RestartBoxHandler,
			"aio:destroy":   DestroyBoxHandler,
			"aio:configure": ConfigureBoxHandler,
		},
	}
}

func (s *BoxerSuite) TearDownTest(c *C) {
	s.Redis.Do("FLUSHDB")
	s.Redis.Close()
}

// TODO: check if the quota is set correctly
func (s *BoxerSuite) TestCreateBox(c *C) {
	message, _ := s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	c.Assert(message.Type, Equals, "aio:created")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Port, Equals, "10002")
	c.Assert(message.Success, Equals, true)

	root := "/lxc/disk1/mclovins-container-1"
	var checkExists = []string{
		"/var/lib/lxc/mclovins-container-1",
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
		"/etc/projects":                    {"1:/lxc/disk1/mclovins-container-1"},
		root + "/rootfs/etc/passwd":        {"action:x:15001:15001"},
		root + "/rootfs/etc/action/box_id": {"1"},
		root + "/rootfs/etc/hostname":      {"mclovins-container-1"},
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-key QWER",
			"ssh-key ASDF",
		},
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

	// actually mount the container at /var/lib/lxc after create at /lxc/disk1
	isMounted, err := Mounted("/var/lib/lxc/mclovins-container-1")
	c.Assert(err, IsNil)
	c.Assert(isMounted, Equals, true)

	// check the container contents at /lxc/disk1
	DoesExist(c, checkExists)
	DoesContain(c, checkContainsContent)
	DoesHavePermissions(c, checkPermissions)

	// check redis
	usedIp, _ := redis.String(s.Redis.Do("LPOP", "aio:used_resources"))
	c.Assert(usedIp, Equals, "192.168.254.2")
}

func (s *BoxerSuite) TestStop(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	message, _ := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:stop",
		Name:  "mclovins-container-1",
	})
	defer s.deleteBox("1", "mclovins-container")

	c.Assert(message.Type, Equals, "aio:stopped")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", "mclovins-container-1").CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "STOPPED"), Equals, true)

	DoesNotExist(c, []string{
		"/var/lib/lxc/mclovins-container-1",
	})

	// TODO: do we really need to remove the quota info here
	DoesNotContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/lxc/disk1/mclovins-container-1_delta/rootfs/home/action"},
	})

	box, _ := s.ResourcePool.FindBox("mclovins-container-1")
	c.Assert(box.State, Equals, "stopped")
}

func (s *BoxerSuite) TestStart(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")
	message, _ := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:start",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:started")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", "mclovins-container-1").CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "RUNNING"), Equals, true)

	DoesExist(c, []string{
		"/var/lib/lxc/mclovins-container-1/rootfs",
	})

	DoesContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/lxc/disk1/mclovins-container-1_delta/rootfs/home/action"},
	})

	box, _ := s.ResourcePool.FindBox("mclovins-container-1")
	c.Assert(box.State, Equals, "running")
}

func (s *BoxerSuite) TestDestroy(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	message, _ := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:destroy",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:destroyed")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", "mclovins-container-1").CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "STOPPED"), Equals, true)

	DoesNotExist(c, []string{
		"/var/lib/lxc/mclovins-container-1",
		"/lxc/disk1/mclovins-container-1",
		"/lxc/disk1/mclovins-container-1_delta",
	})

	DoesNotContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/var/lib/lxc/mclovins-container-1_delta/rootfs/home/action"},
	})

	output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
	c.Assert(strings.Contains(string(output), "192.168.254.2:22"), Equals, false)

	// check redis
	availableIp, _ := redis.String(s.Redis.Do("RPOP", "aio:available_resources"))
	c.Assert(availableIp, Equals, "192.168.254.2")
}

func (s *BoxerSuite) TestRestart(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")
	message, _ := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:restart",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:restarted")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	stdout, _ := exec.Command("sudo", "lxc-info", "-n", "mclovins-container-1").CombinedOutput()
	c.Assert(strings.Contains(string(stdout), "RUNNING"), Equals, true)

	DoesExist(c, []string{
		"/var/lib/lxc/mclovins-container-1",
		"/lxc/disk1/mclovins-container-1",
		"/lxc/disk1/mclovins-container-1_delta",
	})

	DoesContain(c, map[string][]string{
		"/etc/projid":   []string{"mclovins-container-1:1"},
		"/etc/projects": []string{"1:/lxc/disk1/mclovins-container-1_delta/rootfs/home/action"},
	})

	box, _ := s.ResourcePool.FindBox("mclovins-container-1")
	c.Assert(box.State, Equals, "running")
}

func (s *BoxerSuite) TestAddPublicKey(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message, _ := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:add_public_key",
		Data:     "ssh-rsa publickey",
		Name:     "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root := "/lxc/disk1/mclovins-container-1"

	var checkContainsContent = map[string][]string{
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-rsa QWER",
			"ssh-rsa ASDF",
			"ssh-rsa publickey",
		},
	}

	DoesContain(c, checkContainsContent)
}

func (s *BoxerSuite) TestRemovePublicKey(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message, _ := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:remove_public_key",
		Data:     "ssh-rsa QWER",
		Name:     "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root := "/lxc/disk1/mclovins-container-1"
	var checkContent = map[string][]string{
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-rsa QWER",
		},
	}
	DoesNotContain(c, checkContent)
}

func (s *BoxerSuite) TestChangeBoxSpecs(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")
	limit, _ := ViewQuotaLimit("1", "/lxc/disk1")
	c.Assert(limit.Limit, Equals, 1024*1024)

	message, _ := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:change_specs",
		Data: map[string]interface{}{
			"memory":    "512M",
			"diskSpace": "2G",
		},
		Name: "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root := "/lxc/disk1/mclovins-container-1"
	var checkContent = map[string][]string{
		root + "/config": {
			"lxc.cgroup.memory.limit_in_bytes = 512M",
			"lxc.cgroup.memory.memsw.limit_in_bytes = 1536M",
		},
	}
	DoesContain(c, checkContent)

	newLimit, _ := ViewQuotaLimit("1", "/lxc/disk1")
	c.Assert(newLimit.Limit, Equals, 2*1024*1024)

	box, _ := s.ResourcePool.FindBox("mclovins-container-1")
	c.Assert(box.State, Equals, "running")
}
