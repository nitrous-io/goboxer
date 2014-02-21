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
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/s3"
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"

	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	"ext4": testProvisionStrategy{
		MountMethod:  "aufs",
		QuotaMethod:  "usrquota",
		SelectedDisk: "/lxc/disk2",
		SelectedVG:   "",
	},
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
	c.Assert(strings.Contains(info, state), Equals, true, Commentf("Expected state %s; lxc-info says '%s'", state, strings.TrimSpace(info)))
}

func (s *BoxerSuite) mergeProvisionStrategy(p testProvisionStrategy) {
	s.Boxer.MountMethod = p.MountMethod
	s.Boxer.SelectedDisk = p.SelectedDisk
	s.Boxer.QuotaMethod = p.QuotaMethod
	s.Boxer.SelectedVG = p.SelectedVG
	s.Boxer.S3Auth = s3.AuthTest{}
	s.Boxer.S3Region = "ap-southeast-1"
	s.Boxer.S3Bucket = "staging-bucket"
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

type messageOverrideFn func(msg *Message)

// defaultMsg is a no-op for messageOverrideFn
func defaultMsg(msg *Message) { return }

// makeMessage creates a message with default values.  Callers can pass a
// messageOverrideFn to set custom values
func (s *BoxerSuite) makeMessage(messageType string, override messageOverrideFn) Message {
	m := Message{
		Id:    "testboxer",
		BoxId: 1,
		Name:  "mclovins-container-1",
	}
	m.Type = messageType
	override(&m)
	return m
}

func (s *BoxerSuite) makeCreateMessage(override messageOverrideFn) Message {
	m := s.makeMessage("aio:create", func(msg *Message) {
		msg.Name = "mclovins-container"
		msg.Runtime = "test"
		msg.Memory = "256M"
		msg.DiskSpace = "1G"
		msg.PublicKeys = []string{"ssh-rsa QWER", "ssh-rsa ASDF"}
	})
	override(&m)
	return m
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
		boxes, _ := s.ResourcePool.FindAllBoxes()
		for _, box := range boxes {
			box.boxer = s.Boxer
			box.Destroy()
		}
	}
}

func (s *BoxerSuite) receiveMessages(name string, messages ...Message) (Message, *Box) {
	var returnMsg Message
	for _, msg := range messages {
		returnMsg = s.Boxer.Receive(msg)
		// sleep for some time here to prevent lxc choking when we execute
		// stuff too fast
		time.Sleep(500 * time.Millisecond)
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

func (s *BoxerSuite) TestBoxCPUQuota(c *C) {
	box := &Box{}
	c.Assert(s.Boxer.BoxCPUQuota(box), Equals, 0.5)
	box = &Box{Premium: true}
	c.Assert(s.Boxer.BoxCPUQuota(box), Equals, 1.0)
}

func (s *BoxerSuite) TestBoxCPUQuotaDefaults(c *C) {
	// Create a new Boxer with no CPU quotas configured.
	s.Boxer = &Boxer{}

	box := &Box{}
	c.Assert(s.Boxer.BoxCPUQuota(box), Equals, 0.0)
	box = &Box{Premium: true}
	c.Assert(s.Boxer.BoxCPUQuota(box), Equals, 0.0)
}

func (s *BoxerSuite) TestCreateBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) { msg.CpuShares = 2048 }))

		c.Assert(message.Type, Equals, "aio:created")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Port, Equals, 10002)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa QWER", "ssh-rsa ASDF"})

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
				root + "/rootfs/home/action/README.md",
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
				root + "/rootfs/etc/network/interfaces": {
					"address 192.168.254.2",
					"gateway 192.168.254.1",
				},
				root + "/config": {
					"lxc.cgroup.memory.limit_in_bytes = 256M",
					"lxc.cgroup.memory.memsw.limit_in_bytes = 512M",
					"lxc.network.hwaddr = 02:00",
					"lxc.cgroup.cpu.shares = 2048",
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

func (s *BoxerSuite) TestCreateWithSnapshot(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1", s.makeCreateMessage(
			func(msg *Message) {
				msg.CpuShares = 2048
				msg.SnapshotUrl = "s3://ap-southeast-1/staging-apse1.action.io/testboxer-1234.tar.gz"
			}))

		c.Assert(message.Type, Equals, "aio:created")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Port, Equals, 10002)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa QWER", "ssh-rsa ASDF"})

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
				// because the stub s3 data has it
				root + "/rootfs/home/action/test.txt",
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
				root + "/rootfs/etc/network/interfaces": {
					"address 192.168.254.2",
					"gateway 192.168.254.1",
				},
				root + "/config": {
					"lxc.cgroup.memory.limit_in_bytes = 256M",
					"lxc.cgroup.memory.memsw.limit_in_bytes = 512M",
					"lxc.network.hwaddr = 02:00",
					"lxc.cgroup.cpu.shares = 2048",
				},
			},
			Permissions: map[string][]int{
				root + "/rootfs/home/action":                 {15001, 15001},
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

func (s *BoxerSuite) TestCreateWithDefaultCpuShares(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", s.makeCreateMessage(defaultMsg))
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.shares = 512"},
		})
	})
}

func (s *BoxerSuite) TestCreateWithVirtualMemory(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", s.makeCreateMessage(
			func(msg *Message) {
				msg.VirtualMemory = "1024"
			}))
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
		_, box := s.receiveMessages("mclovins-container-1", s.makeCreateMessage(
			func(msg *Message) {
				msg.Premium = true
			}))

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
		})

		b, _ := s.ResourcePool.FindBox("mclovins-container-1")
		c.Assert(b.Premium, Equals, true)
	})
}

func (s *BoxerSuite) TestStop(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg), s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:stop", defaultMsg))

		c.Assert(message.Type, Equals, "aio:stopped")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.State, Equals, "stopped")
		DoesNotExist(c, []string{
			box.LxcPath(),
		})

		AssertIptablesOutput(c, "192.168.254.2:22", false)
	})
}

func (s *BoxerSuite) TestStart(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		s.assertBoxState(c, box, "running")
		DoesExist(c, []string{
			box.LxcPath() + "/rootfs",
			"/var/log/containers/mclovins-container-1/" + box.MountName() + ".lxc-start.log",
		})
	})
}

func (s *BoxerSuite) TestStartSetsPublicKeys(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
			},
		})
	})
}

func (s *BoxerSuite) TestStartAcceptsPublicKeys(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) { msg.PublicKeys = []string{"ssh-rsa first"} }),
			s.makeMessage("aio:start", func(msg *Message) {
				msg.PublicKeys = []string{"ssh-rsa second", "ssh-rsa third"}
			}))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa second",
				"ssh-rsa third",
			},
		})
		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa second", "ssh-rsa third"})
	})
}

func (s *BoxerSuite) TestStartSetsPublicKeysOnStoppedBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) { msg.PublicKeys = []string{"ssh-rsa 1"} }),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {"ssh-rsa 1"},
		})
		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa 1"})

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:start", func(msg *Message) {
				msg.PublicKeys = []string{"ssh-rsa 2", "ssh-rsa 3"}
			}))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		DoesNotContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {"ssh-rsa 1"},
		})
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa 2",
				"ssh-rsa 3",
			},
		})
		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa 2", "ssh-rsa 3"})
	})
}

// Tests that starting a box that has no public keys in its record will read
// and save the keys from the public keys file in the box's rootfs.
func (s *BoxerSuite) TestStartPersistsPublicKeysFromFile(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		// First, we start a box with no public keys.
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) { msg.PublicKeys = []string{} }),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		DoesExist(c, []string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys",
		})
		c.Assert(box.PublicKeys, DeepEquals, []string{})

		// Then, manually add public keys to the box's public key file to simulate
		// an old box from before we started persisting public keys.
		box.PublicKeys = []string{"ssh-rsa QWER", "ssh-rsa ASDF"}
		box.WritePublicKeys()
		box.PublicKeys = []string(nil) // Reset to no keys in Redis.
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
			},
		})

		// Then, we stop and start the box and verify that it's now consistent, i.e.
		// 1. public keys are written to file, and
		// 2. the same public keys are persisted.
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
			},
		})
		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa QWER", "ssh-rsa ASDF"})
	})
}

func (s *BoxerSuite) TestStartSetsIptablesRule(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", s.makeCreateMessage(defaultMsg))
		box.unsetPortForwarding()

		AssertIptablesOutput(c, "192.168.254.2:22", false)

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Success, Equals, true)
		s.assertBoxState(c, box, "running")

		AssertIptablesOutput(c, "192.168.254.2:22", true)
	})
}

func (s *BoxerSuite) TestStartWithNewSpecs(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		mockData := make(map[string]interface{})
		json.Unmarshal(
			[]byte("{\"memory\":\"640M\",\"virtualMemory\":\"1280M\",\"cpuShares\":1536}"),
			&mockData)

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", func(msg *Message) { msg.Data = mockData }))

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

func (s *BoxerSuite) TestStartSetsCpuSet(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		// Explicitly set cpu set on Boxer to make results consistent.
		// This cannot be more than the number of CPUs where this test is run
		// otherwise lxc won't be able to start the container.
		s.Boxer.SetCpuSet(1)

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus = 0"},
		})
	})
}

func (s *BoxerSuite) TestStartWithDefaultCPUQuotaNotConfigured(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		s.Boxer.DefaultCPUQuota = 0.0

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		// Make sure CPU quota configs aren't set.
		DoesNotContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"cfs_period_us",
				"cfs_quota_us",
			},
		})
	})
}

func (s *BoxerSuite) TestStartWithPremiumCPUQuotaNotConfigured(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		s.Boxer.PremiumCPUQuota = 0.0

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": true,
				}
			}),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		// Make sure CPU quota configs aren't set.
		DoesNotContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"cfs_period_us",
				"cfs_quota_us",
			},
		})
	})
}

func (s *BoxerSuite) TestDestroy(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg))
		containerPlace := []string{box.LxcPath()}
		containerPlace = append(containerPlace, box.ContainerFS())
		containerPlace = append(containerPlace, box.ContainerDeltaPath())

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:destroy", defaultMsg))

		c.Assert(box, IsNil)
		c.Assert(message.Type, Equals, "aio:destroyed")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesNotExist(c, containerPlace)

		AssertIptablesOutput(c, "192.168.254.2:22", false)

		// check redis
		conn := s.Redis.Get()
		defer conn.Close()
		availableIp, _ := redis.String(conn.Do("RPOP", "aio:available_resources"))
		c.Assert(availableIp, Equals, "192.168.254.2")
	})
}

func (s *BoxerSuite) TestDestroyWithDeeplyNestedPath(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))
		// create a deeply nested path
		paths := []string{box.ContainerDeltaPath()}
		for i := 0; i < 1000; i++ {
			paths = append(paths, "holla")
		}
		path := filepath.Join(paths...)
		exec.Command("mkdir", "-p", path).Run()

		message, _ = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:destroy", defaultMsg))
		c.Assert(message.Type, Equals, "aio:destroyed")
		c.Assert(message.Success, Equals, true)
	})
}

func (s *BoxerSuite) TestRestart(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:restart", defaultMsg))

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		s.assertBoxState(c, box, "running")

		containerPlace := []string{box.LxcPath()}
		containerPlace = append(containerPlace, box.ContainerDeltaPath())
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
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:restart", func(msg *Message) { msg.Data = mockData }))

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

func (s *BoxerSuite) TestRestartSetsCpuSet(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		// This cannot be more than the number of CPUs where this test is run
		// otherwise lxc won't be able to start the container.
		s.Boxer.SetCpuSet(1)

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus = 0"},
		})

		// Manually remove cpuset.cpus config from the lxc config file so we can
		// verify that restarting a box will set it.
		box.CpuSet = []int{}
		box.writeConfig()
		DoesNotContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus"},
		})

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:restart", defaultMsg))

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus = 0"},
		})
	})
}

func (s *BoxerSuite) TestRestartNewlyPremiumBox(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": true,
				}
			}))

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:update_premium")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
		})

		// Now restart the box to verify that the cfs_quota_us changes.
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:restart", defaultMsg))

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
		})

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestRestartNewlyFreeBox(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": true,
				}
			}),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
		})

		// Now make the box a free box and restart it to verify that the cfs_quota_us is reduced.
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": false,
				}
			}),
			s.makeMessage("aio:restart", defaultMsg))

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
			box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
		})

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestStopStartNewlyPremiumBox(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": true,
				}
			}))

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
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

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
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": true,
				}
			}))

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:update_premium")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/action/premium": {"1"},
		})

		_, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": false,
				}
			}))

		DoesNotExist(c, []string{
			box.LxcPath() + "/rootfs/etc/action/premium",
		})
	})
}

func (s *BoxerSuite) TestAddPublicKey(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:add_public_key"
				msg.Data = "ssh-rsa publickey"
			}))

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:add_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.PublicKeys, DeepEquals, []string{
			"ssh-rsa QWER",
			"ssh-rsa ASDF",
			"ssh-rsa publickey",
		})
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
				"ssh-rsa publickey",
			},
		})
	})
}

func (s *BoxerSuite) TestAddPublicKeyToStoppedBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) {
				msg.PublicKeys = []string{"ssh-rsa QWER"}
			}),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:add_public_key"
				msg.Data = "ssh-rsa newkey"
			}))
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:add_public_key")
		c.Assert(box.State, Equals, "stopped")
		DoesNotExist(c, []string{
			box.LxcPath(),
		})

		// Public keys are persisted to Redis and won't be available on the
		// filesystem since a stopped box's root fs won't be mounted.
		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa QWER", "ssh-rsa newkey"})
	})
}

func (s *BoxerSuite) TestAddPublicKeyStripsNewlines(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) {
				msg.PublicKeys = []string{"ssh-rsa QWER"}
			}),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:add_public_key"
				msg.Data = "ssh-rsa publickey\n\n"
			}))

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:add_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.PublicKeys, DeepEquals, []string{
			"ssh-rsa QWER",
			"ssh-rsa publickey",
		})
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa publickey",
			},
		})
	})
}

func (s *BoxerSuite) TestRemovePublicKey(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa QWER", "ssh-rsa ASDF"})
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {
				"ssh-rsa QWER",
				"ssh-rsa ASDF",
			},
		})

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:remove_public_key"
				msg.Data = "ssh-rsa QWER"
			}))

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:remove_public_key")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa ASDF"})
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {"ssh-rsa ASDF"},
		})
		DoesNotContain(c, map[string][]string{
			box.LxcPath() + "/rootfs/etc/ssh/univ_public_keys": {"ssh-rsa QWER"},
		})
	})
}

func (s *BoxerSuite) TestRemovePublicKeyFromStoppedBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:remove_public_key"
				msg.Data = "ssh-rsa QWER"
			}))
		defer box.Destroy()

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.DataType, Equals, "aio:configured:remove_public_key")
		c.Assert(box.State, Equals, "stopped")
		DoesNotExist(c, []string{
			box.LxcPath(),
		})

		// Public keys are persisted to Redis.
		c.Assert(box.PublicKeys, DeepEquals, []string{"ssh-rsa ASDF"})
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsPass(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg))

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
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:change_specs"
				msg.Data = mockData
			}))

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
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsPassOnRunningBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"384M\"}"),
			&mockData)
		_, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:change_specs"
				msg.Data = mockData
			}))

		s.assertBoxState(c, box, "running")
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 384M",
			},
		})
		newMemLimit, err := lxc.CgroupGet(box.MountName(), "memory.limit_in_bytes")
		c.Assert(err, IsNil)
		c.Assert(strings.TrimSpace(newMemLimit), Equals,
			strconv.Itoa(quota.ConvertValueUnitToByte(quota.ValueUnit{384, "M"})*1024))
		newTotalLimit, err := lxc.CgroupGet(box.MountName(), "memory.memsw.limit_in_bytes")
		c.Assert(err, IsNil)
		c.Assert(strings.TrimSpace(newTotalLimit), Equals,
			strconv.Itoa(quota.ConvertValueUnitToByte(quota.ParseValueUnit(box.TotalMemory()))*1024))
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsPassOnStoppedBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"384M\",\"diskSpace\":\"2048M\",\"cpuShares\":2048}"),
			&mockData)
		_, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:change_specs"
				msg.Data = mockData
			}))

		s.assertBoxState(c, box, "stopped")
		_, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:start", defaultMsg))
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 384M",
			},
		})
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
	c.Assert(m.Runtimes, HasLen, 2)
	c.Assert(SliceInclude(m.Runtimes, "ruby"), Equals, true)
	c.Assert(SliceInclude(m.Runtimes, "test"), Equals, true)
	c.Assert(m.FreeMemory, NotNil)
	c.Assert(m.CPUs, NotNil)
}

func (s *BoxerSuite) TestNewAdvertiseMessageBoxStats(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(func(msg *Message) {
				msg.Premium = true
			}),
			s.makeMessage("aio:start", defaultMsg),
		)
		c.Check(message.Type, Equals, "aio:started")
		c.Check(message.Success, Equals, true)

		s.insertResource("192.168.254.3", "192.168.254.1", "br-lxc-254", "10003")
		message, _ = s.receiveMessages("mclovins-container-2",
			s.makeCreateMessage(func(msg *Message) {
				msg.BoxId = 2
			}),
		)
		c.Check(message.Type, Equals, "aio:created")
		c.Check(message.Success, Equals, true)

		m := s.Boxer.NewAdvertiseMessage()

		c.Assert(m.BoxStats, HasLen, 2)
		c.Assert(m.BoxStats["total"], Equals, 2)
		c.Assert(m.BoxStats["running"], Equals, 1)
	})
}

func (s *BoxerSuite) TestStartAll(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		s.insertResource("192.168.254.3", "192.168.254.1", "br-lxc-254", "10003")

		boxes := []*Box{}
		for _, id := range []int{1, 2} {
			var b *Box
			_, b = s.receiveMessages("mclovins-container-"+strconv.Itoa(id),
				s.makeCreateMessage(func(msg *Message) {
					msg.BoxId = id
				}))
			boxes = append(boxes, b)
		}
		for _, box := range boxes {
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
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:info_request", defaultMsg))

		c.Assert(message.Type, Equals, "aio:info_response")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		c.Assert(message.Info.LxcStatus, Equals, "RUNNING")
		c.Assert(message.Info.MemoryUsage > 1.0, Equals, true)

		message, _ = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:info_request", defaultMsg))
		c.Assert(message.Type, Equals, "aio:info_response")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)
		c.Assert(message.Info.LxcStatus, Equals, "STOPPED")
		c.Assert(message.Info.MemoryUsage, Equals, -1.0)
	})
}

func (s *BoxerSuite) TestSetEnvVariables(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:set_env_variables"
				msg.Data = map[string]interface{}{
					"RDS_HOST":     "mysql://insecure",
					"RDS_PASSWORD": "mysql-insecure",
				}
			}))

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
		_, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:set_env_variables"
				msg.Data = map[string]interface{}{
					"RDS_HOST":     "mysql://insecure",
					"RDS_PASSWORD": "mysql-insecure",
				}
			}),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:set_env_variables"
				msg.Data = map[string]interface{}{
					"RDS_HOST":     "mysql://secure",
					"RDS_PASSWORD": "mysql-insecure",
				}
			}))

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
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:set_env_variables"
				msg.Data = map[string]interface{}{
					"RDS_HOST":     "mysql://insecure",
					"RDS_PASSWORD": "mysql-insecure",
					"REDIS_HOST":   "127.0.0.1",
				}
			}),
			removeMessage)

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
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:set_env_variables"
				msg.Data = []string{
					"so wrong",
					"i dont know what to say",
				}
			}))
		c.Assert(message.Type, Equals, "aio:invalid:params")
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsFail(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg))

		quotaProvisioner := quota.Provisioner.GetQuota(stratValue.QuotaMethod)
		limit, _ := quotaProvisioner.ViewLimit(box.quotaArg())
		c.Log("limit", limit, box.quotaArg(), stratValue.QuotaMethod)
		c.Assert(limit.Limit > 900*1000, Equals, true)

		// swap Quota with FailQuota so all quota calls will return errors
		quota.Provisioner = false

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:change_specs"
				msg.Data = map[string]interface{}{
					"memory":    "512M",
					"diskSpace": "2048M",
				}
			}))

		c.Assert(message.Type, Equals, "aio:configured")
		c.Assert(message.Success, Equals, false)
		c.Assert(message.DataType, Equals, "aio:configured:change_specs")
		data := (message.Data).(map[string]interface{})

		currentMemory, _ := (data["memory"]).(string)
		currentDiskSpace, _ := (data["diskSpace"]).(string)
		// memory is set successfully
		c.Assert(currentMemory, Equals, "512M")
		// disk space cant be set so we still use the old value
		c.Assert(currentDiskSpace, Equals, "1G")

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

func (s *BoxerSuite) TestChangeBoxSpecsInvalidParams(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:change_specs"
				msg.Data = map[string]interface{}{
					"memory":    512,
					"diskSpace": 2048,
				}
			}))

		c.Assert(message.Success, Equals, false)
		data := (message.Data).(map[string]interface{})
		currentMemory, _ := (data["memory"]).(string)
		currentDiskSpace, _ := (data["diskSpace"]).(string)
		c.Assert(currentMemory, Equals, "256M")
		c.Assert(currentDiskSpace, Equals, "1G")
	})
}

func (s *BoxerSuite) TestUpdateAuth(c *C) {
	fakeCartman := s.setupFakeCartman()
	defer fakeCartman.Close()

	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_auth"
				msg.Data = map[string]interface{}{
					"auth_username": "action",
					"auth_password": "password",
				}
			}))

		c.Assert(message.Success, Equals, true)
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_auth"
				msg.Data = map[string]interface{}{
					"auth_username": "invalid_user",
				}
			}))

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
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_auth"
				msg.Data = map[string]interface{}{
					"auth_username": "action",
					"auth_password": "password",
				}
			}))

		c.Assert(message.Success, Equals, true)
		c.Assert(box.AuthUsername, Equals, "action")
		c.Assert(box.AuthPassword, Equals, "password")

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_auth"
				msg.Data = map[string]interface{}{
					"auth_username": nil,
					"auth_password": nil,
				}
			}))

		c.Assert(message.Success, Equals, true)
		c.Assert(box.AuthUsername, Equals, "")
		c.Assert(box.AuthPassword, Equals, "")
	})
}

func (s *BoxerSuite) TestSnapshot(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:snapshot", func(msg *Message) {
				msg.SnapshotId = 123
			}))

		c.Assert(message.Success, Equals, true)
		c.Assert(message.Type, Equals, "aio:snapshotted")
		c.Assert(message.SnapshotId, Equals, 123)
		c.Assert(message.SnapshotUrl, Equals, "s3://staging-bucket/mclovins-container-1-snapshot-123.tar.gz")
		// Test container delta folder should be < 1M in size, so size should be
		// rounded up to 1M.
		c.Assert(message.SnapshotSize, Equals, 1)
	})
}

func (s *BoxerSuite) TestDeleteSnapshot(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:delete_snapshot", func(msg *Message) {
				msg.SnapshotId = 123
				msg.SnapshotUrl = "s3://staging-bucket/mclovins-container-1-snapshot-123.tar.gz"
			}))

		c.Assert(message.Success, Equals, true)
		c.Assert(message.Type, Equals, "aio:deleted_snapshot")
		c.Assert(message.SnapshotId, Equals, 123)

		message, _ = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:delete_snapshot", func(msg *Message) {
				msg.SnapshotId = 124
				msg.SnapshotUrl = "s3://staging-bucket/fail"
			}))

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Type, Equals, "aio:deleted_snapshot")
		c.Assert(message.SnapshotId, Equals, 124)
	})
}
