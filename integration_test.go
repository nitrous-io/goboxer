// Integration test for goboxer
//
// This does not test how goboxer and coach interact with each other, only test
// the behavior within goboxer itself. Therefore, we dont want to involve
// pubsub testing here
package goboxer

import (
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
)

type IntegrationSuite struct {
	Redis        redis.Conn
	Boxer        *Boxer
	ResourcePool *ResourcePool
	Runtime      *Runtime
}

var _ = Suite(&IntegrationSuite{})

func (s *IntegrationSuite) insertResource(ip, gateway, networkInterface, port string) {
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	s.Redis.Do("RPUSH", "aio:available_resources", ip)
	s.Redis.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *IntegrationSuite) SetUpTest(c *C) {
	s.Redis, _ = redis.Dial("tcp", ":65000")
	s.Redis.Do("Select", 13)
	s.Redis.Do("FLUSHDB")
	s.insertResource("192.168.254.2", "192.168.254.1", "br-lxc-254", "10002")

	s.Boxer = &Boxer{
		Root:         "/var/lib/lxc",
		Disks:        []string{"/lxc/disk1"},
		SelectedDisk: "/lxc/disk1",
		Runtimes: map[string]string{
			"test": "test_container_template",
		},
	}

	s.ResourcePool = &ResourcePool{Conn: s.Redis}

	s.Runtime = &Runtime{
		Id:           "testboxer",
		Boxer:        s.Boxer,
		ResourcePool: s.ResourcePool,
		Handles: map[string]MessageHandler{
			"aio:locate":  LocateBoxHandler,
			"aio:create":  CreateBoxHandler,
			"aio:start":   StartBoxHandler,
			"aio:stop":    StopBoxHandler,
			"aio:restart": RestartBoxHandler,
			"aio:destroy": DestroyBoxHandler,
		},
	}
}

func (s *IntegrationSuite) TearDownTest(c *C) {
	s.Redis.Do("FLUSHDB")
	s.Redis.Close()
}

// TODO: check if the quota is set correctly
func (s *IntegrationSuite) TestCreateBox(c *C) {
	message, _ := s.Runtime.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer DestroyTestBox("mclovins-container-1", "192.168.254.2")

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
