// Integration test for gober
//
// This does not test how goboxer and coach interact with each other, only test
// the behavior within goboxer itself. Therefore, we dont want to involve
// pubsub testing here
package goboxer

import (
	"encoding/json"
	"fmt"
	"github.com/action-io/goboxer/cartman"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type BoxerSuite struct {
	Redis        *redis.Pool
	Boxer        *Boxer
	ResourcePool *ResourcePool
}

type testProvisionStrategy struct {
	MountMethod  string
	QuotaMethod  string
	SelectedDisk string
	SelectedVG   string
}

// for each strategies, we are going to run all operations to make sure that
// everything works
var strategies = map[string]testProvisionStrategy{
	"xfs": testProvisionStrategy{
		MountMethod:  "aufs",
		QuotaMethod:  "prjquota",
		SelectedDisk: "/lxc/disk1",
		SelectedVG:   "",
	},
	"ext4": testProvisionStrategy{
		MountMethod:  "aufs",
		QuotaMethod:  "usrquota",
		SelectedDisk: "/lxc/disk2",
		SelectedVG:   "",
	},
	"lvm": testProvisionStrategy{
		MountMethod:  "lvm",
		QuotaMethod:  "lvm",
		SelectedDisk: "",
		SelectedVG:   "aio_boxhost",
	},
}

func cleanContainer(containerType, containerName, disk, uid, selectedVG string) error {
	switch containerType {
	case "ext4":
		script, _ := filepath.Abs("script/support/boxer-clean-ext4")
		cmd := exec.Command(script, containerName, disk, uid)
		cmd.Dir = "script/support"
		return cmd.Run()
	case "xfs":
		script, _ := filepath.Abs("script/support/boxer-clean-xfs")
		cmd := exec.Command(script, containerName, disk)
		cmd.Dir = "script/support"
		return cmd.Run()
	case "lvm":
		script, _ := filepath.Abs("script/support/boxer-clean-lvm")
		cmd := exec.Command(script, containerName, disk, uid, selectedVG)
		cmd.Dir = "script/support"
		return cmd.Run()
	}
	return fmt.Errorf("%s is not a recognized container type", containerType)
}

var _ = Suite(&BoxerSuite{})

func (s *BoxerSuite) insertResource(ip, gateway, networkInterface, port string) {
	conn := s.Redis.Get()
	defer conn.Close()
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	conn.Do("RPUSH", "aio:available_resources", ip)
	conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *BoxerSuite) assertBoxState(c *C, containerName, state string) {
	box, _ := s.ResourcePool.FindBox(containerName)
	c.Assert(box.State, Equals, state)
}

func (s *BoxerSuite) mergeProvisionStrategy(p testProvisionStrategy) {
	s.Boxer.MountMethod = p.MountMethod
	s.Boxer.SelectedDisk = p.SelectedDisk
	s.Boxer.QuotaMethod = p.QuotaMethod
	s.Boxer.SelectedVG = p.SelectedVG
	s.Boxer.Configure()
}

type testFunction func(s *BoxerSuite, c *C, stratName string, stratValue testProvisionStrategy)

func (s *BoxerSuite) testEachStrategy(c *C, test testFunction) {
	for stratName, stratValue := range strategies {
		conn := s.Redis.Get()
		conn.Do("FLUSHDB")
		s.insertResource("192.168.254.2", "192.168.254.1", "br-lxc-254", "10002")
		conn.Close()

		s.mergeProvisionStrategy(stratValue)
		test(s, c, stratName, stratValue)
	}
}

func (s *BoxerSuite) deleteBox(id int, name string) Message {
	message := s.Boxer.Receive(Message{
		Type:  "aio:destroy",
		Id:    "testboxer",
		BoxId: id,
		Name:  name + "-" + strconv.Itoa(id),
	})
	return message
}

func (s *BoxerSuite) SetUpTest(c *C) {
	s.Redis = NewRedisPool(":6379")
	s.ResourcePool = &ResourcePool{Pool: s.Redis}
	s.Boxer = &Boxer{
		Id:           "testboxer",
		Root:         "/var/lib/lxc",
		SelectedDisk: "/lxc/disk1",
		Disks:        []string{"/lxc/disk1"},
		Runtimes:     map[string]string{"test": "test_container_template"},
		ResourcePool: s.ResourcePool,
	}
}

func (s *BoxerSuite) TearDownTest(c *C) {
	conn := s.Redis.Get()
	defer conn.Close()
	defer s.Redis.Close()
	conn.Do("FLUSHDB")
}

func (s *BoxerSuite) TestCreateBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		message := s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
			CpuShares:  2048,
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		c.Assert(message.Type, Equals, "aio:created")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Port, Equals, 10002)
		c.Assert(message.Success, Equals, true)

		root := "/var/lib/lxc/mclovins-container-1"
		var checkExists = []string{
			root,
			root + "/rootfs",
			root + "/rootfs/home/action/workspace",
			root + "/rootfs/home/action/.ssh",
			root + "/rootfs/home/action/.ssh/id_rsa",
			root + "/rootfs/home/action/.ssh/id_rsa.pub",
		}

		var notExists = []string{
			root + "/rootfs/etc/action/premium",
		}

		var checkContainsContent = map[string][]string{
			root + "/rootfs/etc/passwd":          {"action:x:15001:15001"},
			root + "/rootfs/etc/action/box_id":   {"1"},
			root + "/rootfs/etc/action/box_user": {"action"},
			root + "/rootfs/etc/hostname":        {"mclovins-container-1"},
			root + "/rootfs/etc/hosts":           {"mclovins-container-1"},
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
				"lxc.cgroup.memory.memsw.limit_in_bytes = 512M",
				"lxc.cgroup.memory.swappiness = 60",
				"lxc.network.hwaddr = 02:00",
				"lxc.cgroup.cpu.shares = 2048",
			},
		}

		var checkPermissions = map[string][]int{
			root + "/rootfs/home/action":                 {15001, 15001},
			root + "/rootfs/home/action/workspace":       {15001, 15001},
			root + "/rootfs/home/action/.ssh":            {15001, 15001},
			root + "/rootfs/home/action/.ssh/id_rsa":     {15001, 15001},
			root + "/rootfs/home/action/.ssh/id_rsa.pub": {15001, 15001},
		}

		// actually mount the container at /var/lib/lxc after create
		isMounted, err := mount.Mounted("/var/lib/lxc/mclovins-container-1")
		c.Assert(err, IsNil)
		c.Assert(isMounted, Equals, true)

		// check the container contents
		DoesExist(c, checkExists)
		DoesNotExist(c, notExists)
		DoesContain(c, checkContainsContent)
		DoesHavePermissions(c, checkPermissions)

		// check redis
		conn := s.Redis.Get()
		defer conn.Close()
		usedIp, _ := redis.String(conn.Do("LPOP", "aio:used_resources"))
		c.Assert(usedIp, Equals, "192.168.254.2")
	})
}

func (s *BoxerSuite) TestCreateWithDefaultCpuShares(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		root := "/var/lib/lxc/mclovins-container-1"
		var checkContent = map[string][]string{
			root + "/config": {
				"lxc.cgroup.cpu.shares = 512",
			},
		}
		DoesContain(c, checkContent)
	})
}

func (s *BoxerSuite) TestCreateWithVirtualMemory(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:            "testboxer",
			BoxId:         1,
			Type:          "aio:create",
			Name:          "mclovins-container",
			Runtime:       "test",
			Memory:        "256M",
			DiskSpace:     "1G",
			VirtualMemory: "1024",
			PublicKeys:    []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		root := "/var/lib/lxc/mclovins-container-1"
		// total memory = memory + virtual memory
		var checkContent = map[string][]string{
			root + "/config": {
				"lxc.cgroup.memory.memsw.limit_in_bytes = 257M",
			},
		}
		DoesContain(c, checkContent)

		// virtualMemory is persisted with redis
		b, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(b.VirtualMemory, Equals, "1024")
		c.Assert(b.TotalMemory(), Equals, "257M")
	})
}

func (s *BoxerSuite) TestCreatePremiumBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			Premium:    true,
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		root := "/var/lib/lxc/mclovins-container-1"
		var checkContent = map[string][]string{
			root + "/rootfs/etc/action/premium": {"1"},
		}
		DoesContain(c, checkContent)

		// virtualMemory is persisted with redis
		b, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(b.Premium, Equals, true)
	})
}

func (s *BoxerSuite) TestStop(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:stop",
			Name:  "mclovins-container-1",
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		c.Assert(message.Type, Equals, "aio:stopped")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		AssertLxcState(c, "mclovins-container-1", "STOPPED")

		DoesNotExist(c, []string{
			"/var/lib/lxc/mclovins-container-1",
		})

		s.assertBoxState(c, "mclovins-container-1", "stopped")
	})
}

func (s *BoxerSuite) TestStart(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		AssertLxcState(c, "mclovins-container-1", "RUNNING")

		DoesExist(c, []string{
			"/var/lib/lxc/mclovins-container-1/rootfs",
		})

		s.assertBoxState(c, "mclovins-container-1", "running")
	})
}

func (s *BoxerSuite) TestStartWithNewSpecs(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		// mock data
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"640M\",\"virtualMemory\":\"1280M\",\"cpuShares\":1536}"),
			&mockData)

		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Data:  mockData,
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		AssertLxcState(c, "mclovins-container-1", "RUNNING")

		root := "/var/lib/lxc/mclovins-container-1"
		DoesExist(c, []string{
			root + "/rootfs",
		})

		DoesContain(c, map[string][]string{
			root + "/config": []string{
				"lxc.cgroup.memory.limit_in_bytes = 640M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1920M",
				"lxc.cgroup.cpu.shares = 1536",
			},
		})

		// virtualMemory is persisted with redis
		b, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(b.VirtualMemory, Equals, "1280M")
		c.Assert(b.CpuShares, Equals, 1536)
		c.Assert(b.Memory, Equals, "640M")

		s.assertBoxState(c, "mclovins-container-1", "running")
	})
}

func (s *BoxerSuite) TestDestroy(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:destroy",
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:destroyed")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		AssertLxcState(c, "mclovins-container-1", "STOPPED")

		containerPlace := []string{
			"/var/lib/lxc/mclovins-container-1",
		}
		switch stratName {
		case "xfs", "ext4":
			containerPlace = append(containerPlace, stratValue.SelectedDisk+
				"/mclovins-container-1")
			containerPlace = append(containerPlace, stratValue.SelectedDisk+
				"/mclovins-container-1_delta")
		case "lvm":
			containerPlace = append(containerPlace, "/dev/"+stratValue.SelectedVG+
				"/mclovins-container-1")
		}
		DoesNotExist(c, containerPlace)

		output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
		c.Assert(strings.Contains(string(output), "192.168.254.2:22"), Equals, false)

		// check redis
		conn := s.Redis.Get()
		defer conn.Close()
		availableIp, _ := redis.String(conn.Do("RPOP", "aio:available_resources"))
		c.Assert(availableIp, Equals, "192.168.254.2")
		// box should be deleted from the collection
		res, _ := conn.Do("HGET", "aio:boxes", "mclovins-container-1")
		c.Assert(res, IsNil)
	})
}

func (s *BoxerSuite) TestRestart(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:restart",
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		AssertLxcState(c, "mclovins-container-1", "RUNNING")

		containerPlace := []string{
			"/var/lib/lxc/mclovins-container-1",
		}
		switch stratName {
		case "xfs", "ext4":
			containerPlace = append(containerPlace, stratValue.SelectedDisk+
				"/mclovins-container-1")
			containerPlace = append(containerPlace, stratValue.SelectedDisk+
				"/mclovins-container-1_delta")
		case "lvm":
			containerPlace = append(containerPlace, "/dev/"+stratValue.SelectedVG+
				"/mclovins-container-1")
		}
		DoesExist(c, containerPlace)

		s.assertBoxState(c, "mclovins-container-1", "running")
	})
}

func (s *BoxerSuite) TestRestartWithNewSpec(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		// mock data
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"640M\",\"virtualMemory\":\"1280M\",\"cpuShares\":1536}"),
			&mockData)

		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:restart",
			Data:  mockData,
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		AssertLxcState(c, "mclovins-container-1", "RUNNING")

		root := "/var/lib/lxc/mclovins-container-1"
		containerPlace := []string{root}
		switch stratName {
		case "xfs", "ext4":
			containerPlace = append(containerPlace, stratValue.SelectedDisk+
				"/mclovins-container-1")
			containerPlace = append(containerPlace, stratValue.SelectedDisk+
				"/mclovins-container-1_delta")
		case "lvm":
			containerPlace = append(containerPlace, "/dev/"+stratValue.SelectedVG+
				"/mclovins-container-1")
		}
		DoesExist(c, containerPlace)

		DoesExist(c, []string{
			root + "/rootfs",
		})

		DoesContain(c, map[string][]string{
			root + "/config": []string{
				"lxc.cgroup.memory.limit_in_bytes = 640M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1920M",
				"lxc.cgroup.cpu.shares = 1536",
			},
		})

		// virtualMemory is persisted with redis
		b, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(b.VirtualMemory, Equals, "1280M")
		c.Assert(b.CpuShares, Equals, 1536)
		c.Assert(b.Memory, Equals, "640M")

		s.assertBoxState(c, "mclovins-container-1", "running")
	})
}

func (s *BoxerSuite) TestUpdatePremium(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Hello, boxer")
		}))
	defer ts.Close()
	// simulate response from cartman server
	cartmanURL, _ := url.Parse(ts.URL)
	s.Boxer.Cartman = &cartman.Cartman{Host: cartmanURL.Host}

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_premium",
			Data: map[string]interface{}{
				"premium": true,
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:update_premium")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		root := "/var/lib/lxc/mclovins-container-1"
		var checkContainsContent = map[string][]string{
			root + "/rootfs/etc/action/premium": {"1"},
		}
		DoesContain(c, checkContainsContent)

		s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_premium",
			Data: map[string]interface{}{
				"premium": false,
			},
			Name: "mclovins-container-1",
		})

		DoesNotExist(c, []string{
			root + "/rootfs/etc/action/premium",
		})
	})
}

func (s *BoxerSuite) TestAddPublicKey(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:add_public_key",
			Data:     "ssh-rsa publickey",
			Name:     "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:add_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		root := "/var/lib/lxc/mclovins-container-1"

		var checkContainsContent = map[string][]string{
			root + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
				"ssh-rsa publickey",
			},
		}

		DoesContain(c, checkContainsContent)
	})
}

func (s *BoxerSuite) TestRemovePublicKey(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:remove_public_key",
			Data:     "ssh-rsa QWER",
			Name:     "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:remove_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		root := "/var/lib/lxc/mclovins-container-1"
		var checkContent = map[string][]string{
			root + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
			},
		}
		DoesNotContain(c, checkContent)
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsPass(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		quotaProvisioner := quota.Provisioner.GetQuota(stratValue.QuotaMethod)
		root := "/var/lib/lxc/mclovins-container-1"
		arg := quota.QuotaArg{
			Id:   "1",
			Disk: stratValue.SelectedDisk,
			Path: root + "_delta",
		}
		limit, _ := quotaProvisioner.ViewLimit(arg)
		c.Assert(limit.Limit > 900*1000, Equals, true)

		// mock data
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"512M\",\"diskSpace\":\"2048M\",\"cpuShares\":2048}"),
			&mockData)

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:change_specs",
			Data:     mockData,
			Name:     "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		c.Assert(message.DataType, Equals, "aio:configured:change_specs")

		data := (message.Data).(map[string]interface{})
		currentMemory, _ := (data["memory"]).(string)
		currentDiskSpace, _ := (data["diskSpace"]).(string)
		c.Assert(currentMemory, Equals, "512M")
		c.Assert(currentDiskSpace, Equals, "2048M")

		var checkContent = map[string][]string{
			root + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 512M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1024M",
				"lxc.cgroup.cpu.shares = 2048",
			},
		}
		DoesContain(c, checkContent)

		newLimit, _ := quotaProvisioner.ViewLimit(arg)
		c.Assert(newLimit.Limit > 2*900*1000, Equals, true)

		s.assertBoxState(c, "mclovins-container-1", "running")
	})
}

func (s *BoxerSuite) TestNewAdvertiseMessage(c *C) {
	m := s.Boxer.NewAdvertiseMessage()
	c.Assert(m.Runtimes, DeepEquals, []string{"test"})
	s.Boxer.Runtimes = map[string]string{
		"test": "test_container_template",
		"ruby": "ruby_template",
	}
	m = s.Boxer.NewAdvertiseMessage()
	c.Assert(len(m.Runtimes), Equals, 2)
	c.Assert(SliceInclude(m.Runtimes, "ruby"), Equals, true)
	c.Assert(SliceInclude(m.Runtimes, "test"), Equals, true)
}

func (s *BoxerSuite) TestStartAll(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		s.insertResource("192.168.254.3", "192.168.254.1", "br-lxc-254", "10003")

		for _, id := range []int{1, 2} {
			cleanContainer(stratName, "mclovins-container-"+strconv.Itoa(id),
				stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
			s.Boxer.Receive(Message{
				Id:         "testboxer",
				BoxId:      id,
				Type:       "aio:create",
				Name:       "mclovins-container",
				Runtime:    "test",
				Memory:     "256M",
				DiskSpace:  "1G",
				PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
			})
			defer cleanContainer(stratName,
				"mclovins-container-"+strconv.Itoa(id),
				stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		}

		AssertLxcState(c, "mclovins-container-1", "STOPPED")
		AssertLxcState(c, "mclovins-container-2", "STOPPED")

		box1, _ := s.ResourcePool.FindBox("mclovins-container-1")
		box2, _ := s.ResourcePool.FindBox("mclovins-container-2")
		s.Boxer.StartAll([]*Box{box1, box2})

		AssertLxcState(c, "mclovins-container-1", "RUNNING")
		AssertLxcState(c, "mclovins-container-2", "RUNNING")

		DoesExist(c, []string{
			"/var/lib/lxc/mclovins-container-1/rootfs",
			"/var/lib/lxc/mclovins-container-2/rootfs",
		})

		s.assertBoxState(c, "mclovins-container-1", "running")
		s.assertBoxState(c, "mclovins-container-2", "running")
	})
}

func (s *BoxerSuite) TestInfo(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Name:  "mclovins-container-1",
		})

		message := s.Boxer.Receive(Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:info_request",
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:info_response")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(message.Info.LxcStatus, Equals, "RUNNING")
	})
}

func (s *BoxerSuite) TestSetEnvVariables(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			Name:     "mclovins-container-1",
			DataType: "aio:configure:set_env_variables",
			Data: map[string]interface{}{
				"RDS_HOST":     "mysql://insecure",
				"RDS_PASSWORD": "mysql-insecure",
			},
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:set_env_variables")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		root := "/var/lib/lxc/mclovins-container-1"
		DoesContain(c, map[string][]string{
			root + "/rootfs/etc/environment": []string{
				"RDS_HOST=mysql://insecure",
				"RDS_PASSWORD=mysql-insecure",
			},
		})
	})
}

func (s *BoxerSuite) TestDoubleSetEnvVariables(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})

		s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			Name:     "mclovins-container-1",
			DataType: "aio:configure:set_env_variables",
			Data: map[string]interface{}{
				"RDS_HOST":     "mysql://insecure",
				"RDS_PASSWORD": "mysql-insecure",
			},
		})

		s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			Name:     "mclovins-container-1",
			DataType: "aio:configure:set_env_variables",
			Data: map[string]interface{}{
				"RDS_HOST":     "mysql://secure",
				"RDS_PASSWORD": "mysql-insecure",
			},
		})

		root := "/var/lib/lxc/mclovins-container-1"
		DoesContain(c, map[string][]string{
			root + "/rootfs/etc/environment": []string{
				"RDS_HOST=mysql://secure",
				"RDS_PASSWORD=mysql-insecure",
			},
		})
		DoesNotContain(c, map[string][]string{
			root + "/rootfs/etc/environment": []string{
				"RDS_HOST=mysql://insecure",
			},
		})
	})
}

func (s *BoxerSuite) TestRemoveEnvVariables(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})

		s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			Name:     "mclovins-container-1",
			DataType: "aio:configure:set_env_variables",
			Data: map[string]interface{}{
				"RDS_HOST":     "mysql://insecure",
				"RDS_PASSWORD": "mysql-insecure",
				"REDIS_HOST":   "127.0.0.1",
			},
		})
		msg := "{\"id\":\"testboxer\",\"name\":\"mclovins-container-1\"," +
			"\"data\":[\"RDS_HOST\", \"RDS_PASSWORD\"]," +
			"\"dataType\":\"aio:configure:remove_env_variables\"" +
			",\"type\":\"aio:configure\"}"
		message := Message{}
		json.Unmarshal([]byte(msg), &message)

		message = s.Boxer.Receive(message)
		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:remove_env_variables")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		root := "/var/lib/lxc/mclovins-container-1"
		DoesContain(c, map[string][]string{
			root + "/rootfs/etc/environment": []string{
				"REDIS_HOST=127.0.0.1",
			},
		})
		DoesNotContain(c, map[string][]string{
			root + "/rootfs/etc/environment": []string{
				"RDS_HOST=mysql://insecure",
				"RDS_PASSWORD=mysql-insecure",
			},
		})
	})
}

func (s *BoxerSuite) TestInvalidConfigureDataDoesNotPanic(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			Name:     "mclovins-container-1",
			DataType: "aio:configure:set_env_variables",
			Data: []string{
				"so wrong",
				"i dont know what to say",
			},
		})

		c.Assert(message.Type, Equals, "aio:invalid:params")
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsFail(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		quotaProvisioner := quota.Provisioner.GetQuota(stratValue.QuotaMethod)
		root := "/var/lib/lxc/mclovins-container-1"
		arg := quota.QuotaArg{
			Id:   "1",
			Disk: stratValue.SelectedDisk,
			Path: root + "_delta",
		}
		limit, _ := quotaProvisioner.ViewLimit(arg)
		// approximate the limit as lvm strategy will not report the whole
		// absolute size as per lvm volume size
		c.Assert(limit.Limit > 900*1000, Equals, true)

		// swap Quota with FailQuota so all quota calls will return errors
		quota.Provisioner = false

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:change_specs",
			Data: map[string]interface{}{
				"memory":    "512M",
				"diskSpace": "2048M",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.Success, Equals, false)
		c.Assert(message.DataType, Equals, "aio:configured:change_specs")
		data := (message.Data).(map[string]interface{})

		currentMemory, _ := (data["memory"]).(string)
		currentDiskSpace, _ := (data["diskSpace"]).(string)
		// memory is set successfully
		c.Assert(currentMemory, Equals, "512M")
		// disk space cant be set so we still use the old value
		c.Assert(currentDiskSpace, Equals, "1024M")

		// Swap back the original Quota so the next test that uses Quota will be
		// normal
		quota.Provisioner = true

		// it still sets the new memory
		var checkContent = map[string][]string{
			root + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 512M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1024M",
			},
		}
		DoesContain(c, checkContent)

		// it cant set the new quota
		newLimit, _ := quotaProvisioner.ViewLimit(arg)
		c.Assert(newLimit.Limit, Equals, limit.Limit)
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsLvmFail(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		if stratName != "lvm" {
			return
		}
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "30M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		quotaProvisioner := quota.Provisioner.GetQuota(stratValue.QuotaMethod)
		root := "/var/lib/lxc/mclovins-container-1"
		arg := quota.QuotaArg{
			Id:   "1",
			Disk: stratValue.SelectedDisk,
			Path: root + "_delta",
		}
		limit, _ := quotaProvisioner.ViewLimit(arg)

		// write 16M to test
		exec.Command("dd", "if=/dev/zero",
			"of=/var/lib/lxc/mclovins-container-1/test_file",
			"bs=1024", "count=16384").Run()

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:change_specs",
			Data: map[string]interface{}{
				"memory":    "512M",
				"diskSpace": "15M",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.Success, Equals, false)

		// it cant set the new quota
		newLimit, _ := quotaProvisioner.ViewLimit(arg)
		c.Assert(newLimit.Limit, Equals, limit.Limit)
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsInvalidParams(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:change_specs",
			Data: map[string]interface{}{
				"memory":    512,
				"diskSpace": 2048,
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, false)
		data := (message.Data).(map[string]interface{})
		currentMemory, _ := (data["memory"]).(string)
		currentDiskSpace, _ := (data["diskSpace"]).(string)
		c.Assert(currentMemory, Equals, "256M")
		c.Assert(currentDiskSpace, Equals, "1024M")
	})
}

func (s *BoxerSuite) TestUpdateAuth(c *C) {
	// simulate response from cartman server
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Hello, boxer")
		}))
	defer ts.Close()
	cartmanURL, _ := url.Parse(ts.URL)
	s.Boxer.Cartman = &cartman.Cartman{Host: cartmanURL.Host}

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)
		defer cleanContainer(stratName, "mclovins-container-1",
			stratValue.SelectedDisk, "15001", stratValue.SelectedVG)

		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})

		message := s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_auth",
			Data: map[string]interface{}{
				"auth_username": "action",
				"auth_password": "password",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, true)

		// check redis
		box, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")

		message = s.Boxer.Receive(Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_auth",
			Data: map[string]interface{}{
				"auth_username": "invalid_user",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Type, Equals, "aio:invalid:params")

		// check redis
		box, _ = s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")
	})
}
