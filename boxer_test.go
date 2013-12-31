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
	// "lvm": testProvisionStrategy{
	// 	MountMethod:  "lvm",
	// 	QuotaMethod:  "lvm",
	// 	SelectedDisk: "",
	// 	SelectedVG:   "aio_boxhost",
	// },
}

type boxContentSpec struct {
	Exists      []string
	NotExists   []string
	Contains    map[string][]string
	NotContains map[string][]string
	Permissions map[string][]int
}

func (b boxContentSpec) Check(c *C) {
	if len(b.Exists) > 0 {
		DoesExist(c, b.Exists)
	}
	if len(b.NotExists) > 0 {
		DoesNotExist(c, b.NotExists)
	}
	if len(b.Contains) > 0 {
		DoesContain(c, b.Contains)
	}
	if len(b.NotContains) > 0 {
		DoesNotContain(c, b.NotContains)
	}
	if len(b.Permissions) > 0 {
		DoesHavePermissions(c, b.Permissions)
	}
}

var _ = Suite(&BoxerSuite{})

func (s *BoxerSuite) insertResource(ip, gateway, networkInterface, port string) {
	conn := s.Redis.Get()
	defer conn.Close()
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	conn.Do("RPUSH", "aio:available_resources", ip)
	conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *BoxerSuite) assertBoxState(c *C, b *Box, state string) {
	state = strings.ToLower(state)
	c.Assert(b.State, Equals, state)
	stdout, _ := exec.Command("sudo", "lxc-info", "-n", b.MountName()).CombinedOutput()
	info := strings.ToLower(string(stdout))
	c.Assert(strings.Contains(info, state), Equals, true)
}

func (s *BoxerSuite) mergeProvisionStrategy(p testProvisionStrategy) {
	s.Boxer.MountMethod = p.MountMethod
	s.Boxer.SelectedDisk = p.SelectedDisk
	s.Boxer.QuotaMethod = p.QuotaMethod
	s.Boxer.SelectedVG = p.SelectedVG
	s.Boxer.Configure()
}

func (s *BoxerSuite) findBox(name string) *Box {
	box, _ := s.ResourcePool.FindBox(name)
	if box != nil {
		box.boxer = s.Boxer
	}
	return box
}

// Returns a fake Cartman server.
// The caller should call Close when finished, to shut down the server.
func (s *BoxerSuite) setupFakeCartman() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Hello, boxer")
		}))
	cartmanURL, _ := url.Parse(server.URL)
	s.Boxer.Cartman = &cartman.Cartman{Host: cartmanURL.Host}
	return server
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

func (s *BoxerSuite) receiveMessages(name string, messages ...Message) (Message, *Box) {
	var returnMsg Message
	for _, msg := range messages {
		returnMsg = s.Boxer.Receive(msg)
	}
	box := s.findBox(name)
	return returnMsg, box
}

func (s *BoxerSuite) SetUpTest(c *C) {
	s.Redis = NewRedisPool(":6379")
	s.ResourcePool = &ResourcePool{Pool: s.Redis}
	s.Boxer = &Boxer{
		Id:              "testboxer",
		Root:            "/var/lib/lxc",
		SelectedDisk:    "/lxc/disk1",
		Disks:           []string{"/lxc/disk1"},
		Runtimes:        map[string]string{"test": "test_container_template"},
		ResourcePool:    s.ResourcePool,
		DefaultCPUQuota: 0.5,
		PremiumCPUQuota: 1.0,
	}
}

func (s *BoxerSuite) TearDownTest(c *C) {
	conn := s.Redis.Get()
	defer conn.Close()
	defer s.Redis.Close()
	conn.Do("FLUSHDB")
}

func (s *BoxerSuite) TestSetCpuSet(c *C) {
	s.Boxer.SetCpuSet(3)
	c.Assert(s.Boxer.CpuSet, DeepEquals, []int{1, 2})
	s.Boxer.SetCpuSet(2)
	c.Assert(s.Boxer.CpuSet, DeepEquals, []int{1})
	s.Boxer.SetCpuSet(1)
	c.Assert(s.Boxer.CpuSet, DeepEquals, []int{0})
}

func (s *BoxerSuite) TestCreateBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
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
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:created")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Port, Equals, 10002)
		c.Assert(message.Success, Equals, true)

		root := box.LxcPath()
		c.Assert(root, Equals, "/var/lib/lxc/mclovins-container-1-"+
			box.UniqueId)

		// actually mount the container at /var/lib/lxc after create
		isMounted, _ := mount.Mounted(root)
		c.Assert(isMounted, Equals, true)

		spec := boxContentSpec{
			Exists: []string{
				root,
				root + "/rootfs",
				root + "/rootfs/home/action/workspace",
				root + "/rootfs/home/action/.ssh",
				root + "/rootfs/home/action/.ssh/id_rsa",
				root + "/rootfs/home/action/.ssh/id_rsa.pub",
			},
			NotExists: []string{
				root + "/rootfs/etc/action/premium",
			},
			Contains: map[string][]string{
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
					"lxc.cgroup.cpu.cfs_period_us = 1000000",
					"lxc.cgroup.cpu.cfs_quota_us = 500000",
				},
			},
			Permissions: map[string][]int{
				root + "/rootfs/home/action":                 {15001, 15001},
				root + "/rootfs/home/action/workspace":       {15001, 15001},
				root + "/rootfs/home/action/.ssh":            {15001, 15001},
				root + "/rootfs/home/action/.ssh/id_rsa":     {15001, 15001},
				root + "/rootfs/home/action/.ssh/id_rsa.pub": {15001, 15001},
			},
		}
		spec.Check(c)

		// check redis
		conn := s.Redis.Get()
		defer conn.Close()
		usedIp, _ := redis.String(conn.Do("LPOP", "aio:used_resources"))
		c.Assert(usedIp, Equals, "192.168.254.2")
	})
}

func (s *BoxerSuite) TestCreateBoxWithCpuRange(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		// reset cpu set to make results consistent
		s.Boxer.SetCpuSet(3)
		message, box := s.receiveMessages("mclovins-container-1", Message{
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
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:created")
		c.Assert(message.Id, Equals, "testboxer")
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus = 1,2"},
		})
	})
}

func (s *BoxerSuite) TestCreateWithDefaultCpuShares(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		defer box.Destroy()
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.shares = 512"},
		})
	})
}

func (s *BoxerSuite) TestCreateWithVirtualMemory(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", Message{
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
		defer box.Destroy()
		// total memory = memory + virtual memory
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.memory.memsw.limit_in_bytes = 257M"},
		})
		// virtualMemory is persisted with redis
		c.Assert(box.VirtualMemory, Equals, "1024")
		c.Assert(box.TotalMemory(), Equals, "257M")
	})
}

func (s *BoxerSuite) TestCreatePremiumBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", Message{
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
		defer box.Destroy()

		var checkContent = map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
		}
		DoesContain(c, checkContent)

		b, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(b.Premium, Equals, true)
	})
}

func (s *BoxerSuite) TestStop(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:stop",
			Name:  "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:stopped")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.State, Equals, "stopped")
		DoesNotExist(c, []string{
			box.LxcPath(),
		})
	})
}

func (s *BoxerSuite) TestStart(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Name:  "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		s.assertBoxState(c, box, "running")
		DoesExist(c, []string{
			box.LxcPath() + "/rootfs",
		})
	})
}

func (s *BoxerSuite) TestStartWithNewSpecs(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		// mock data
		mockData := make(map[string]interface{})
		json.Unmarshal(
			[]byte("{\"memory\":\"640M\",\"virtualMemory\":\"1280M\",\"cpuShares\":1536}"),
			&mockData)
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Data:  mockData,
			Name:  "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		root := box.LxcPath()
		spec := boxContentSpec{
			Exists: []string{root + "/rootfs"},
			Contains: map[string][]string{
				root + "/config": []string{
					"lxc.cgroup.memory.limit_in_bytes = 640M",
					"lxc.cgroup.memory.memsw.limit_in_bytes = 1920M",
					"lxc.cgroup.cpu.shares = 1536",
				},
			},
		}
		spec.Check(c)

		// virtualMemory is persisted with redis
		c.Assert(box.VirtualMemory, Equals, "1280M")
		c.Assert(box.CpuShares, Equals, 1536)
		c.Assert(box.Memory, Equals, "640M")

		// assert that the box is running
		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestDestroy(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		})
		containerPlace := []string{box.LxcPath()}
		switch stratName {
		case "xfs", "ext4":
			containerPlace = append(containerPlace, box.ContainerFS())
			containerPlace = append(containerPlace, box.ContainerDeltaPath())
		case "lvm":
			containerPlace = append(containerPlace, "/dev/"+stratValue.SelectedVG+
				"/mclovins-container-1")
		}

		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:destroy",
			Name:  "mclovins-container-1",
		})

		c.Assert(box, IsNil)
		c.Assert(message.Type, Equals, "aio:destroyed")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesNotExist(c, containerPlace)

		output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
		c.Assert(strings.Contains(string(output), "192.168.254.2:22"), Equals, false)

		// check redis
		conn := s.Redis.Get()
		defer conn.Close()
		availableIp, _ := redis.String(conn.Do("RPOP", "aio:available_resources"))
		c.Assert(availableIp, Equals, "192.168.254.2")
	})
}

func (s *BoxerSuite) TestRestart(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:restart",
			Name:  "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		s.assertBoxState(c, box, "running")

		containerPlace := []string{box.LxcPath()}
		switch stratName {
		case "xfs", "ext4":
			containerPlace = append(containerPlace, box.ContainerDeltaPath())
		case "lvm":
			containerPlace = append(containerPlace, "/dev/"+stratValue.SelectedVG+
				"/mclovins-container-1")
		}
		DoesExist(c, containerPlace)
	})
}

func (s *BoxerSuite) TestRestartWithNewSpec(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		// mock data
		mockData := make(map[string]interface{})
		json.Unmarshal(
			[]byte("{\"memory\":\"640M\",\"virtualMemory\":\"1280M\",\"cpuShares\":1536}"),
			&mockData)
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:restart",
			Data:  mockData,
			Name:  "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": []string{
				"lxc.cgroup.memory.limit_in_bytes = 640M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1920M",
				"lxc.cgroup.cpu.shares = 1536",
			},
		})

		// virtualMemory is persisted with redis
		c.Assert(box.VirtualMemory, Equals, "1280M")
		c.Assert(box.CpuShares, Equals, 1536)
		c.Assert(box.Memory, Equals, "640M")

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestRestartNewlyPremiumBox(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:        "testboxer",
			BoxId:     1,
			Type:      "aio:create",
			Name:      "mclovins-container",
			Runtime:   "test",
			Memory:    "256M",
			DiskSpace: "1G",
		}, Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_premium",
			Data: map[string]interface{}{
				"premium": true,
			},
			Name: "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:update_premium")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
		})

		// Now restart the box to verify that the cfs_quota_us changes.
		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:restart",
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
		})

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestStopStartNewlyPremiumBox(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:        "testboxer",
			BoxId:     1,
			Type:      "aio:create",
			Name:      "mclovins-container",
			Runtime:   "test",
			Memory:    "256M",
			DiskSpace: "1G",
		}, Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_premium",
			Data: map[string]interface{}{
				"premium": true,
			},
			Name: "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:update_premium")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
		})

		// Now stop and then start the box to verify that the cfs_quota_us changes.
		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:stop",
			Name:  "mclovins-container-1",
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Name:  "mclovins-container-1",
		})

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config":                    {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
		})

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestUpdatePremium(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_premium",
			Data: map[string]interface{}{
				"premium": true,
			},
			Name: "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:update_premium")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		c.Assert(box.CpuQuota, Equals, 1.0)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
		})

		_, box = s.receiveMessages("mclovins-container-1", Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_premium",
			Data: map[string]interface{}{
				"premium": false,
			},
			Name: "mclovins-container-1",
		})

		c.Assert(box.CpuQuota, Equals, 0.5)

		DoesNotExist(c, []string{
			box.LxcPath() + "/rootfs/etc/action/premium",
		})
	})
}

func (s *BoxerSuite) TestAddPublicKey(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			Message{
				Id:         "testboxer",
				BoxId:      1,
				Type:       "aio:create",
				Name:       "mclovins-container",
				Runtime:    "test",
				Memory:     "256M",
				DiskSpace:  "1G",
				PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
			}, Message{
				Id:       "testboxer",
				BoxId:    1,
				Type:     "aio:configure",
				DataType: "aio:configure:add_public_key",
				Data:     "ssh-rsa publickey",
				Name:     "mclovins-container-1",
			})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:add_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
				"ssh-rsa publickey",
			},
		})
	})
}

func (s *BoxerSuite) TestRemovePublicKey(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:remove_public_key",
			Data:     "ssh-rsa QWER",
			Name:     "mclovins-container-1",
		})
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:remove_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesNotContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
			},
		})
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsPass(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
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
		limit, _ := quotaProvisioner.ViewLimit(quota.QuotaArg{
			Id:   "1",
			Disk: stratValue.SelectedDisk,
			Path: box.LxcPath() + "_delta",
		})
		c.Assert(limit.Limit > 900*1000, Equals, true)

		// mock data
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"512M\",\"diskSpace\":\"2048M\",\"cpuShares\":2048}"),
			&mockData)
		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:change_specs",
			Data:     mockData,
			Name:     "mclovins-container-1",
		})
		defer box.Destroy()

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

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 512M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1024M",
				"lxc.cgroup.cpu.shares = 2048",
			},
		})

		newLimit, _ := quotaProvisioner.ViewLimit(quota.QuotaArg{
			Id:   "1",
			Disk: stratValue.SelectedDisk,
			Path: box.LxcPath() + "_delta",
		})
		c.Assert(newLimit.Limit > 2*900*1000, Equals, true)

		s.assertBoxState(c, box, "running")
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

		boxes := []*Box{}
		for _, id := range []int{1, 2} {
			var b *Box
			_, b = s.receiveMessages("mclovins-container-"+strconv.Itoa(id),
				Message{
					Id:         "testboxer",
					BoxId:      id,
					Type:       "aio:create",
					Name:       "mclovins-container",
					Runtime:    "test",
					Memory:     "256M",
					DiskSpace:  "1G",
					PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
				})
			boxes = append(boxes, b)
		}
		defer func(boxes []*Box) {
			for _, box := range boxes {
				box.Destroy()
			}
		}(boxes)
		for _, box := range boxes {
			logger.Println("checking", box.ContainerName())
			s.assertBoxState(c, box, "stopped")
		}
		s.Boxer.StartAll(boxes)
		for _, box := range boxes {
			s.assertBoxState(c, box, "running")
		}
	})
}

func (s *BoxerSuite) TestInfo(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:start",
			Name:  "mclovins-container-1",
		}, Message{
			Id:    "testboxer",
			BoxId: 1,
			Type:  "aio:info_request",
			Name:  "mclovins-container-1",
		})
		defer box.Destroy()

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
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
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
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:set_env_variables")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/environment": []string{
				"RDS_HOST=mysql://insecure",
				"RDS_PASSWORD=mysql-insecure",
			},
		})
	})
}

func (s *BoxerSuite) TestDoubleSetEnvVariables(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			Name:     "mclovins-container-1",
			DataType: "aio:configure:set_env_variables",
			Data: map[string]interface{}{
				"RDS_HOST":     "mysql://insecure",
				"RDS_PASSWORD": "mysql-insecure",
			},
		}, Message{
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
		defer box.Destroy()

		spec := boxContentSpec{
			Contains: map[string][]string{
				box.LxcPath() + "/rootfs/etc/environment": []string{
					"RDS_HOST=mysql://secure",
					"RDS_PASSWORD=mysql-insecure",
				},
			},
			NotContains: map[string][]string{
				box.LxcPath() + "/rootfs/etc/environment": []string{
					"RDS_HOST=mysql://insecure",
				},
			},
		}
		spec.Check(c)
	})
}

func (s *BoxerSuite) TestRemoveEnvVariables(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		msg := "{\"id\":\"testboxer\",\"name\":\"mclovins-container-1\"," +
			"\"data\":[\"RDS_HOST\", \"RDS_PASSWORD\"]," +
			"\"dataType\":\"aio:configure:remove_env_variables\"" +
			",\"type\":\"aio:configure\"}"
		removeMessage := Message{}
		json.Unmarshal([]byte(msg), &removeMessage)
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
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
		}, removeMessage)
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:remove_env_variables")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		spec := boxContentSpec{
			Contains: map[string][]string{
				box.LxcPath() + "/rootfs/etc/environment": []string{
					"REDIS_HOST=127.0.0.1",
				},
			},
			NotContains: map[string][]string{
				box.LxcPath() + "/rootfs/etc/environment": []string{
					"RDS_HOST=mysql://insecure",
					"RDS_PASSWORD=mysql-insecure",
				},
			},
		}
		spec.Check(c)
	})
}

func (s *BoxerSuite) TestInvalidConfigureDataDoesNotPanic(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
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
		defer box.Destroy()
		c.Assert(message.Type, Equals, "aio:invalid:params")
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsFail(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
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
		limit, _ := quotaProvisioner.ViewLimit(box.quotaArg())
		// approximate the limit as lvm strategy will not report the whole
		// absolute size as per lvm volume size
		c.Assert(limit.Limit > 900*1000, Equals, true)

		// swap Quota with FailQuota so all quota calls will return errors
		quota.Provisioner = false

		message, box = s.receiveMessages("mclovins-container-1", Message{
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
		defer box.Destroy()

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
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 512M",
				"lxc.cgroup.memory.memsw.limit_in_bytes = 1024M",
			},
		})

		// it cant set the new quota
		newLimit, _ := quotaProvisioner.ViewLimit(box.quotaArg())
		c.Assert(newLimit.Limit, Equals, limit.Limit)
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsLvmFail(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		if stratName != "lvm" {
			return
		}
		message, box := s.receiveMessages("mclovins-container-1", Message{
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
		limit, _ := quotaProvisioner.ViewLimit(box.quotaArg())

		// write 16M to test
		exec.Command("dd", "if=/dev/zero",
			"of="+box.LxcPath()+"/test_file",
			"bs=1024", "count=16384").Run()

		message, box = s.receiveMessages("mclovins-container-1", Message{
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
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.Success, Equals, false)

		newLimit, _ := quotaProvisioner.ViewLimit(box.quotaArg())
		c.Assert(newLimit.Limit, Equals, limit.Limit)
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsInvalidParams(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
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
		defer box.Destroy()

		c.Assert(message.Success, Equals, false)
		data := (message.Data).(map[string]interface{})
		currentMemory, _ := (data["memory"]).(string)
		currentDiskSpace, _ := (data["diskSpace"]).(string)
		c.Assert(currentMemory, Equals, "256M")
		c.Assert(currentDiskSpace, Equals, "1024M")
	})
}

func (s *BoxerSuite) TestUpdateAuth(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
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
		defer box.Destroy()

		c.Assert(message.Success, Equals, true)
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")

		message, box = s.receiveMessages("mclovins-container-1", Message{
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
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")
	})
}

func (s *BoxerSuite) TestRemoveAuth(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", Message{
			Id:         "testboxer",
			BoxId:      1,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1024M",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		}, Message{
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
		defer box.Destroy()

		c.Assert(message.Success, Equals, true)
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")

		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_auth",
			Data: map[string]interface{}{
				"auth_username": nil,
				"auth_password": nil,
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, true)
		c.Assert(box.AuthUsername, Equals, "")
		c.Assert(box.AuthPassword, Equals, "")
	})
}
