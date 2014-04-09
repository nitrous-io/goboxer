// Integration test for gober
//
// This does not test how goboxer and coach interact with each other, only test
// the behavior within goboxer itself. Therefore, we dont want to involve
// pubsub testing here
package goboxer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/action-io/goboxer/cartman"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/quota"
	"github.com/action-io/goboxer/s3"
	"github.com/garyburd/redigo/redis"
	. "github.com/motain/gocheck"
)

type BoxerSuite struct {
	Redis        *redis.Pool
	Boxer        *Boxer
	ResourcePool *ResourcePool

	cartmanRequest *http.Request
}

type testProvisionStrategy struct {
	Provisioner  string
	MountMethod  string
	QuotaMethod  string
	SelectedDisk string
	SelectedVG   string
}

// for each strategies, we are going to run all operations to make sure that
// everything works
var strategies = map[string]testProvisionStrategy{
	//"ext4": testProvisionStrategy{
	//    Provisioner:  "lxc",
	//    MountMethod:  "aufs",
	//    QuotaMethod:  "usrquota",
	//    SelectedDisk: "/lxc/disk2",
	//    SelectedVG:   "",
	//},
	"docker": testProvisionStrategy{
		Provisioner:  "docker",
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
	switch b.Provisioner {
	case "lxc":
		lxcState, _ := lxc.Status(b.ContainerPath(), b.MountName())
		c.Assert(lxcState, Equals, strings.ToUpper(state))
	case "docker":
		lxcState, _ := lxc.Status("", b.UniqueId)
		c.Assert(lxcState, Equals, strings.ToUpper(state))
	}
}

func (s *BoxerSuite) assertInvalidateCartman(c *C, b *Box) {
	// assert cartman requests
	c.Assert(s.cartmanRequest.URL.Path, Equals, "/invalidate_cache")
	c.Assert(s.cartmanRequest.URL.Query().Get("id"), Equals,
		b.ContainerName())
	// reset request. Any further access to this without a calls to cartman
	// should fail spectacularly
	s.cartmanRequest = nil
}

func (s *BoxerSuite) mergeProvisionStrategy(p testProvisionStrategy) {
	s.Boxer.MountMethod = p.MountMethod
	s.Boxer.SelectedDisk = p.SelectedDisk
	s.Boxer.QuotaMethod = p.QuotaMethod
	s.Boxer.SelectedVG = p.SelectedVG
	s.Boxer.Provisioner = p.Provisioner
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
			// record cartman request so we can test it
			s.cartmanRequest = r
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
		func() {
			conn := s.Redis.Get()
			conn.Do("FLUSHDB")
			fakeCartman := s.setupFakeCartman()
			defer fakeCartman.Close()
			defer func() {
				boxes, _ := s.ResourcePool.FindAllBoxes()
				for _, box := range boxes {
					box.boxer = s.Boxer
					box.Destroy()
				}
			}()
			s.insertResource("192.168.254.2", "192.168.254.1", "br-lxc-254", "10002")
			conn.Close()

			s.mergeProvisionStrategy(stratValue)
			test(s, c, stratName, stratValue)
		}()
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
		SelectedDisk:    "/lxc/disk1",
		Disks:           []string{"/lxc/disk1"},
		Runtimes:        map[string]string{"test": "test_container_template"},
		ResourcePool:    s.ResourcePool,
		DefaultCPUQuota: 0.5,
		PremiumCPUQuota: 1.0,
	}
	os.MkdirAll(ScratchDir, 0755)
}

func (s *BoxerSuite) TearDownTest(c *C) {
	conn := s.Redis.Get()
	defer conn.Close()
	defer s.Redis.Close()
	conn.Do("FLUSHDB")
	os.RemoveAll(ScratchDir)
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

		switch stratValue.Provisioner {
		case "lxc":
			lxcPath := box.LxcPath()
			c.Assert(lxcPath, Equals, "/lxc/disk2/mclovins-container-1-"+box.UniqueId)

			// actually mount the container at /var/lib/lxc after create
			isMounted, _ := mount.Mounted(lxcPath)
			c.Assert(isMounted, Equals, true)

			// check to see if ssh keys are different from
			// base_container_template. If they are, fail test.
			sshKeyTypes := []string{"rsa", "dsa", "ecdsa"}
			baseContainerPath := filepath.Join(box.Disk, "base_container_template")
			for _, keyType := range sshKeyTypes {
				baseKey, _ := ioutil.ReadFile(filepath.Join(baseContainerPath,
					"rootfs", "etc", "ssh", fmt.Sprintf("ssh_host_%s_key", keyType)))
				containerKey, _ := ioutil.ReadFile(filepath.Join(lxcPath, "rootfs",
					"etc", "ssh", fmt.Sprintf("ssh_host_%s_key", keyType)))
				c.Assert(string(baseKey), Not(Equals), string(containerKey))
			}
		case "docker":
		}

		root := box.RootFS()
		switch stratValue.Provisioner {
		case "lxc":
			spec := boxContentSpec{
				Contains: map[string][]string{
					root + "/etc/network/interfaces": {
						"address 192.168.254.2",
						"gateway 192.168.254.1",
					},
					root + "/config": {
						"lxc.cgroup.memory.limit_in_bytes = 256M",
						"lxc.cgroup.memory.memsw.limit_in_bytes = 384M",
						"lxc.network.hwaddr = 02:00",
						"lxc.cgroup.cpu.shares = 2048",
					},
				},
			}
			spec.Check(c)
		case "docker":
		}

		spec := boxContentSpec{
			Exists: []string{
				root,
				root + "/home/action/workspace",
				root + "/home/action/README.md",
				root + "/home/action/.ssh",
				root + "/home/action/.ssh/id_rsa",
				root + "/home/action/.ssh/id_rsa.pub",
			},
			NotExists: []string{
				root + "/rootfs/etc/action/premium",
			},
			Contains: map[string][]string{
				root + "/etc/passwd":          {"action:x:15001:15001"},
				root + "/etc/action/box_id":   {"1"},
				root + "/etc/action/box_user": {"action"},
				root + "/etc/hostname":        {"mclovins-container-1"},
				root + "/etc/hosts":           {"mclovins-container-1"},
			},
			Permissions: map[string][]int{
				root + "/home/action":                 {15001, 15001},
				root + "/home/action/workspace":       {15001, 15001},
				root + "/home/action/.ssh":            {15001, 15001},
				root + "/home/action/.ssh/id_rsa":     {15001, 15001},
				root + "/home/action/.ssh/id_rsa.pub": {15001, 15001},
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

		root := box.RootFS()
		switch box.Provisioner {
		case "lxc":
			spec := boxContentSpec{
				Contains: map[string][]string{
					root + "/etc/network/interfaces": {
						"address 192.168.254.2",
						"gateway 192.168.254.1",
					},
					root + "/config": {
						"lxc.cgroup.memory.limit_in_bytes = 256M",
						"lxc.cgroup.memory.memsw.limit_in_bytes = 384M",
						"lxc.network.hwaddr = 02:00",
						"lxc.cgroup.cpu.shares = 2048",
					},
				},
			}
			spec.Check(c)
		}

		spec := boxContentSpec{
			Exists: []string{
				root,
				// because the stub s3 data has it
				root + "/home/action/test.txt",
				root + "/home/action/.ssh",
				root + "/home/action/.ssh/id_rsa",
				root + "/home/action/.ssh/id_rsa.pub",
			},
			NotExists: []string{
				root + "/rootfs/etc/action/premium",
			},
			Contains: map[string][]string{
				root + "/etc/passwd":          {"action:x:15001:15001"},
				root + "/etc/action/box_id":   {"1"},
				root + "/etc/action/box_user": {"action"},
				root + "/etc/hostname":        {"mclovins-container-1"},
				root + "/etc/hosts":           {"mclovins-container-1"},
			},
			Permissions: map[string][]int{
				root + "/home/action":                 {15001, 15001},
				root + "/home/action/workspace":       {15001, 15001},
				root + "/home/action/.ssh":            {15001, 15001},
				root + "/home/action/.ssh/id_rsa":     {15001, 15001},
				root + "/home/action/.ssh/id_rsa.pub": {15001, 15001},
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

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.shares = 512"},
			})
		}

	})
}

func (s *BoxerSuite) TestCreateWithVirtualMemory(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		_, box := s.receiveMessages("mclovins-container-1", s.makeCreateMessage(
			func(msg *Message) {
				msg.VirtualMemory = "1024"
			}))
		switch box.Provisioner {
		case "lxc":
			// total memory = memory + virtual memory
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.memory.memsw.limit_in_bytes = 257M"},
			})
		}

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
			box.RootFS() + "/etc/action/premium": {"1"},
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

		switch box.Provisioner {
		case "lxc":
			DoesNotExist(c, []string{
				box.LxcPath(),
			})
		}

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
		switch box.Provisioner {
		case "lxc":
			DoesExist(c, []string{
				"/var/log/containers/mclovins-container-1/" + box.ContainerName() + ".lxc-start.log",
			})
		}
		DoesExist(c, []string{
			box.RootFS(),
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {"ssh-rsa 1"},
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {"ssh-rsa 1"},
		})
		DoesContain(c, map[string][]string{
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys",
		})
		c.Assert(box.PublicKeys, DeepEquals, []string{})

		// Then, manually add public keys to the box's public key file to simulate
		// an old box from before we started persisting public keys.
		box.PublicKeys = []string{"ssh-rsa QWER", "ssh-rsa ASDF"}
		box.WritePublicKeys()
		box.PublicKeys = []string(nil) // Reset to no keys in Redis.
		DoesContain(c, map[string][]string{
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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

		switch box.Provisioner {
		case "lxc":
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
		}

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

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus = 0"},
			})
		}
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

		switch box.Provisioner {
		case "lxc":
			// Make sure CPU quota configs aren't set.
			DoesNotContain(c, map[string][]string{
				box.LxcPath() + "/config": {
					"cfs_period_us",
					"cfs_quota_us",
				},
			})
		}
	})
}

func (s *BoxerSuite) TestStartWithPremiumCPUQuotaNotConfigured(c *C) {
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

		switch box.Provisioner {
		case "lxc":
			// Make sure CPU quota configs aren't set.
			DoesNotContain(c, map[string][]string{
				box.LxcPath() + "/config": {
					"cfs_period_us",
					"cfs_quota_us",
				},
			})
		}
	})
}

func (s *BoxerSuite) TestStartWritesNitrousboxrcFiles(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		DoesContain(c, map[string][]string{
			box.RootFS() + "/etc/init/run-nitrousboxrc.conf": {
				"setuid action",
				"setgid action",
			},
			box.RootFS() + "/home/action/.nitrousboxrc.sample": {
				"cp ~/.nitrousboxrc.sample ~/.nitrousboxrc",
				"chmod +x ~/.nitrousboxrc",
			},
		})
	})
}

func (s *BoxerSuite) TestStartRunsPartsInit(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		// The test box does not have parts installed, so
		// fake it
		partsData := `
#!/bin/bash

echo $@ > /home/action/parts-inited
touch /etc/pam.d/shouldnotexist
`
		partsFile, err := os.Create(filepath.Join(box.RootFS(), "usr", "bin", "parts"))
		c.Assert(err, IsNil)
		partsFile.WriteString(partsData)
		partsFile.Chmod(os.FileMode(0755))
		partsFile.Sync()
		partsFile.Close()
		err = os.RemoveAll(box.RootFS() + "/home/action/parts-inited")
		c.Assert(err, IsNil)

		_, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:restart", defaultMsg))

		initedPath := box.RootFS() + "/home/action/parts-inited"
		RetryEvery(1*time.Second, 10*time.Second, func() bool {
			return exists(initedPath)
		})
		DoesContain(c, map[string][]string{
			initedPath: {
				"init -",
			},
		})
		// check that parts init is running without privileges
		DoesNotExist(c, []string{
			box.RootFS() + "/etc/pam.d/shouldnotexist",
		})
	})
}

func (s *BoxerSuite) TestStartRunsNitrousboxrc(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		rcData := `
#!/bin/bash

echo check logging
touch filetouch # check pwd
rm /etc/sudoers # check privileges, stderr logging`
		rcFile, err := os.Create(filepath.Join(box.RootFS(), "home", "action", ".nitrousboxrc"))
		c.Assert(err, IsNil)
		rcFile.WriteString(rcData)
		rcFile.Chmod(os.FileMode(0755))
		rcFile.Sync()
		rcFile.Close()
		err = os.RemoveAll(box.RootFS() + "/home/action/nitrousboxrc.log")
		c.Assert(err, IsNil)

		_, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:restart", defaultMsg))

		logPath := box.RootFS() + "/home/action/.nitrousboxrc.log"
		RetryEvery(1*time.Second, 10*time.Second, func() bool {
			return exists(logPath)
		})
		DoesContain(c, map[string][]string{
			logPath: {
				"Starting /home/action/.nitrousboxrc",
				"check logging",
				"cannot remove `/etc/sudoers': Permission denied",
				"Finished /home/action/.nitrousboxrc",
			},
		})
		logData, _ := ioutil.ReadFile(logPath)
		fileContent := string(logData)
		startCount := strings.Count(fileContent, "Starting")
		c.Assert(startCount, Equals, 1, Commentf("Expected .nitrousboxrc to run once, ran %d times", startCount))
		DoesExist(c, []string{
			box.RootFS() + "/home/action/filetouch",
			box.RootFS() + "/etc/sudoers",
		})
	})
}

func (s *BoxerSuite) TestDestroy(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg))
		containerPlace := []string{box.RootFS()}

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
		paths := []string{box.RootFS()}
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

		containerPlace := []string{box.RootFS()}
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

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": []string{
					"lxc.cgroup.memory.limit_in_bytes = 640M",
					"lxc.cgroup.memory.memsw.limit_in_bytes = 1920M",
					"lxc.cgroup.cpu.shares = 1536",
				},
			})
		}

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

		switch box.Provisioner {
		case "lxc":
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
		}

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:restart", defaultMsg))

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpuset.cpus = 0"},
			})
		}

	})
}

func (s *BoxerSuite) TestRestartNewlyPremiumBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(
				func(msg *Message) {
					msg.VirtualMemory = "128M"
				}),
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
		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
			})
		}

		s.assertInvalidateCartman(c, box)

		// When a newly premium box restarts, virtualMemory will be changed
		mockData := make(map[string]interface{})
		json.Unmarshal(
			[]byte("{\"memory\":\"256M\",\"virtualMemory\":\"512M\"}"),
			&mockData)

		// Now restart the box to verify that the cfs_quota_us changes.
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:restart", func(msg *Message) {
				msg.Data = mockData
			}))

		c.Assert(message.Type, Equals, "aio:restarted")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
			})
		}

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestRestartNewlyFreeBox(c *C) {
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
		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
			})
		}

		s.assertInvalidateCartman(c, box)

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
		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
			})

		}
		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestStopStartNewlyPremiumBox(c *C) {
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
		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 500000"},
			})
		}
		DoesContain(c, map[string][]string{
			box.RootFS() + "/etc/action/premium": {"1"},
		})
		s.assertInvalidateCartman(c, box)

		// Now stop and then start the box to verify that the cfs_quota_us changes.
		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:stop", defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Type, Equals, "aio:started")
		c.Assert(message.Id, Equals, "testboxer")
		c.Assert(message.BoxId, Equals, 1)
		c.Assert(message.Success, Equals, true)

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_period_us = 1000000"},
				box.LxcPath() + "/config": {"lxc.cgroup.cpu.cfs_quota_us = 1000000"},
			})
		}
		DoesContain(c, map[string][]string{
			box.RootFS() + "/etc/action/premium": {"1"},
		})

		s.assertBoxState(c, box, "running")
	})
}

func (s *BoxerSuite) TestUpdatePremium(c *C) {
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
			box.RootFS() + "/etc/action/premium": {"1"},
		})

		s.assertInvalidateCartman(c, box)

		_, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:configure", func(msg *Message) {
				msg.DataType = "aio:configure:update_premium"
				msg.Data = map[string]interface{}{
					"premium": false,
				}
			}))

		DoesNotExist(c, []string{
			box.RootFS() + "/etc/action/premium",
		})

		s.assertInvalidateCartman(c, box)
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {
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
			box.RootFS() + "/etc/ssh/univ_public_keys": {"ssh-rsa ASDF"},
		})
		DoesNotContain(c, map[string][]string{
			box.RootFS() + "/etc/ssh/univ_public_keys": {"ssh-rsa QWER"},
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

		switch box.Provisioner {
		case "lxc":
			DoesContain(c, map[string][]string{
				box.LxcPath() + "/config": {
					"lxc.cgroup.memory.limit_in_bytes = 512M",
					"lxc.cgroup.memory.memsw.limit_in_bytes = 768M",
					"lxc.cgroup.cpu.shares = 2048",
				},
			})
		}

		newLimit, _ := quotaProvisioner.ViewLimit(quota.QuotaArg{
			Id:   "1",
			Disk: stratValue.SelectedDisk,
			Path: box.LxcPath() + "_delta",
		})
		c.Assert(newLimit.Limit > 2*900*1000, Equals, true)

		// it persist the new quota
		reloadedBox := s.findBox(box.ContainerName())
		c.Assert(reloadedBox.DiskSpace, Equals, "2048M")
	})
}

func (s *BoxerSuite) TestChangeBoxSpecsPassOnRunningBox(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		var mockData map[string]interface{}
		json.Unmarshal(
			[]byte("{\"memory\":\"384M\",\"diskSpace\":\"2048M\"}"),
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
		newMemLimit, err := lxc.CgroupGet(box.Disk, box.MountName(), "memory.limit_in_bytes")
		c.Assert(err, IsNil)
		c.Assert(strings.TrimSpace(newMemLimit), Equals,
			strconv.Itoa(quota.ConvertValueUnitToByte(quota.ValueUnit{384, "M"})*1024))
		newTotalLimit, err := lxc.CgroupGet(box.Disk, box.MountName(), "memory.memsw.limit_in_bytes")
		c.Assert(err, IsNil)
		c.Assert(strings.TrimSpace(newTotalLimit), Equals,
			strconv.Itoa(quota.ConvertValueUnitToByte(quota.ParseValueUnit(box.TotalMemory()))*1024))

		// it persist the new quota
		reloadedBox := s.findBox(box.ContainerName())
		c.Assert(reloadedBox.DiskSpace, Equals, "2048M")
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

		_, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:start", defaultMsg))
		DoesContain(c, map[string][]string{
			box.LxcPath() + "/config": {
				"lxc.cgroup.memory.limit_in_bytes = 384M",
			},
		})

		// it persist the new quota
		reloadedBox := s.findBox(box.ContainerName())
		c.Assert(reloadedBox.DiskSpace, Equals, "2048M")
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
				"lxc.cgroup.memory.memsw.limit_in_bytes = 768M",
			},
		})

		// it cant set the new quota
		newLimit, _ := quotaProvisioner.ViewLimit(box.quotaArg())
		c.Assert(newLimit.Limit, Equals, limit.Limit)

		// it persist the old quota
		reloadedBox := s.findBox(box.ContainerName())
		c.Assert(reloadedBox.DiskSpace, Equals, "1G")
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
		s.assertInvalidateCartman(c, box)

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

func (s *BoxerSuite) TestUpdateCustomDomain(c *C) {
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
			DataType: "aio:configure:update_custom_domain",
			Data: map[string]interface{}{
				"custom_domain": "wtf.com",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, true)
		c.Assert(box.CustomDomain, Equals, "wtf.com")
		s.assertInvalidateCartman(c, box)

		// check redis
		conn := s.Redis.Get()
		defer conn.Close()
		containerName, _ := redis.String(conn.Do("HGET", "aio:domains", "wtf.com"))
		c.Assert(containerName, Equals, box.ContainerName())

		// update a newer custom domain
		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_custom_domain",
			Data: map[string]interface{}{
				"custom_domain": "another.com",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, true)
		c.Assert(box.CustomDomain, Equals, "another.com")
		s.assertInvalidateCartman(c, box)

		// check redis
		containerName, _ = redis.String(conn.Do("HGET", "aio:domains", "wtf.com"))
		c.Assert(containerName, Equals, "")
		containerName, _ = redis.String(conn.Do("HGET", "aio:domains", "another.com"))
		c.Assert(containerName, Equals, box.ContainerName())

		// remove custom domain
		message, box = s.receiveMessages("mclovins-container-1", Message{
			Id:       "testboxer",
			BoxId:    1,
			Type:     "aio:configure",
			DataType: "aio:configure:update_custom_domain",
			Data: map[string]interface{}{
				"custom_domain": "",
			},
			Name: "mclovins-container-1",
		})

		c.Assert(message.Success, Equals, true)
		c.Assert(box.CustomDomain, Equals, "")
		s.assertInvalidateCartman(c, box)

		// check redis
		containerName, _ = redis.String(conn.Do("HGET", "aio:domains", "another.com"))
		c.Assert(containerName, Equals, "")
		containerName, _ = redis.String(conn.Do("HGET", "aio:domains", ""))
		c.Assert(containerName, Equals, "")
	})
}

func (s *BoxerSuite) TestRemoveAuth(c *C) {
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
		s.assertInvalidateCartman(c, box)

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
		s.assertInvalidateCartman(c, box)
	})
}

func (s *BoxerSuite) TestSnapshot(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:snapshot", func(msg *Message) {
				msg.SnapshotId = 123
			}))

		c.Assert(message.Success, Equals, true)
		c.Assert(message.Type, Equals, "aio:snapshotted")
		c.Assert(message.SnapshotId, Equals, 123)
		c.Assert(message.SnapshotUrl, Equals, "s3://staging-bucket/mclovins-container-1-snapshot-123.tar.gz")

		quotaInfo, err := box.quota().ViewLimit(box.quotaArg())
		c.Check(err, IsNil)
		c.Assert(message.SnapshotSize, Equals, int(math.Ceil(float64(quotaInfo.Used)/1024.0)))
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

func (s *BoxerSuite) TestCloneRepositoryWithWrongScm(c *C) {
	var badRepoPath = filepath.Join(ScratchDir, "fake-bad")
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {
		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg),
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", badRepoPath)
				msg.RepositoryScm = ScmMercurial
			}))

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Type, Equals, "aio:cloned_repository")
		c.Assert(message.Error, Matches, "Repository download failed")
	})
}

func (s *BoxerSuite) TestCloneGitRepository(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Success, Equals, true)

		gitRepoPath := filepath.Join(ScratchDir, "fake-git")
		err := MakeRepo(box, gitRepoPath, ScmGit)
		c.Assert(err, IsNil, Commentf("Error on git fake: %s\n", err))

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", gitRepoPath)
				msg.RepositoryScm = ScmGit
			}))

		c.Assert(message.Error, Equals, "")
		c.Assert(message.Success, Equals, true)
		c.Assert(message.Type, Equals, "aio:cloned_repository")
		c.Assert(message.RepositoryUrl, Equals, fmt.Sprintf("file://%s", gitRepoPath))

		repoRoot := filepath.Join(box.RootFS(), "home", "action", "workspace", "fake-git")
		spec := boxContentSpec{
			Permissions: map[string][]int{
				repoRoot: {15001, 15001},
				filepath.Join(repoRoot, ".gitignore"): {15001, 15001},
			},
		}
		spec.Check(c)
	})
}

func (s *BoxerSuite) TestCloneHgRepository(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Success, Equals, true)

		hgRepoPath := filepath.Join(ScratchDir, "fake-hg")
		err := MakeRepo(box, hgRepoPath, ScmMercurial)
		c.Assert(err, IsNil, Commentf("Error on hg fake: %s\n", err))

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", hgRepoPath)
				msg.RepositoryScm = ScmMercurial
			}))

		c.Assert(message.Error, Equals, "")
		c.Assert(message.Success, Equals, true)
		c.Assert(message.Type, Equals, "aio:cloned_repository")
		c.Assert(message.RepositoryUrl, Equals, fmt.Sprintf("file://%s", hgRepoPath))

		repoRoot := filepath.Join(box.RootFS(), "home", "action", "workspace", "fake-hg")
		spec := boxContentSpec{
			Permissions: map[string][]int{
				repoRoot: {15001, 15001},
				filepath.Join(repoRoot, ".hgignore"): {15001, 15001},
			},
		}
		spec.Check(c)
	})
}

func (s *BoxerSuite) TestCloneGitRepositoryExceedsQuota(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Success, Equals, true)

		gitRepoPath := filepath.Join(ScratchDir, "fake-git")
		err := MakeRepo(box, gitRepoPath, ScmGit)
		c.Assert(err, IsNil, Commentf("Error on git fake: %s\n", err))

		quotaInfo, err := box.quota().ViewLimit(box.quotaArg())
		box.DiskSpace = strconv.Itoa(quotaInfo.Used+1) + "K"
		err = box.SetQuota()
		c.Check(err, IsNil)

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", gitRepoPath)
				msg.RepositoryScm = ScmGit
			}))

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Error, Matches, ".*exceeds available.*")

		c.Assert(exists(filepath.Join(box.ContainerUserHomePath(), "workspace", "fake-git")), Equals, false)
	})
}

func (s *BoxerSuite) TestCloneGitRepositoryExistingDirectory(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))

		c.Assert(message.Success, Equals, true)

		gitRepoPath := filepath.Join(ScratchDir, "fake-git")
		err := MakeRepo(box, gitRepoPath, ScmGit)
		c.Assert(err, IsNil, Commentf("Error on git fake: %s\n", err))

		os.MkdirAll(filepath.Join(box.ContainerUserHomePath(), "workspace", "fake-git", "existing"), 0755)

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", gitRepoPath)
				msg.RepositoryScm = ScmGit
			}))

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Error, Matches, "Destination directory /home/action/workspace/fake-git already exists.*")

		// existing directory should not be deleted
		c.Assert(exists(filepath.Join(box.ContainerUserHomePath(), "workspace", "fake-git", "existing")), Equals, true)
	})
}

func (s *BoxerSuite) TestRepoDownloadArchive(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))
		c.Assert(message.Success, Equals, true)

		// repo.download() expects the reponame to be 3rd-last element of path
		archivePath := filepath.Join(ScratchDir, "autoparts", "archive", "master.tar.gz")
		err := MakeTargz(archivePath)
		defer os.RemoveAll(archivePath)
		c.Assert(err, IsNil, Commentf("Error on fake tar.gz: %s\n", err))

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", archivePath)
				msg.RepositoryScm = ScmTargz
			}))

		c.Assert(message.Error, Equals, "")
		c.Assert(message.Success, Equals, true)
		c.Assert(exists(filepath.Join(box.RootFS(), "tmp", "autoparts.tar.gz")), Equals, false)

		c.Assert(exists(filepath.Join(box.ContainerUserHomePath(), "workspace", "autoparts", "README.md")), Equals, true)
	})
}

func (s *BoxerSuite) TestRepoDownloadArchiveExceedsQuota(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))
		c.Assert(message.Success, Equals, true)

		quotaInfo, err := box.quota().ViewLimit(box.quotaArg())
		box.DiskSpace = strconv.Itoa(quotaInfo.Used+1) + "K"
		err = box.SetQuota()
		c.Check(err, IsNil)

		// repo.download() expects the reponame to be 3rd-last element of path
		archivePath := filepath.Join(ScratchDir, "autoparts", "archive", "master.tar.gz")
		err = MakeTargz(archivePath)
		defer os.RemoveAll(archivePath)
		c.Assert(err, IsNil, Commentf("Error on fake tar.gz: %s\n", err))

		message, box = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", archivePath)
				msg.RepositoryScm = ScmTargz
			}))

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Error, Matches, ".*exceeds available.*")

		c.Assert(exists(filepath.Join(box.ContainerUserHomePath(), "workspace", "autoparts")), Equals, false)
	})
}

func (s *BoxerSuite) TestRepoDownloadEmpty(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, _ := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))
		c.Assert(message.Success, Equals, true)

		// repo.download() expects the reponame to be 3rd-last element of path
		archivePath := filepath.Join(ScratchDir, "autoparts", "archive", "master.tar.gz")
		os.MkdirAll(filepath.Dir(archivePath), 0755)
		err := ioutil.WriteFile(archivePath, []byte(EmptyRepositoryResponse), 0644)
		c.Assert(err, IsNil, Commentf("Error writing file %s", archivePath))

		message, _ = s.receiveMessages("mclovins-container-1",
			s.makeMessage("aio:clone_repository", func(msg *Message) {
				msg.RepositoryUrl = fmt.Sprintf("file://%s", archivePath)
				msg.RepositoryScm = ScmTargz
			}))

		c.Assert(message.Success, Equals, false)
		c.Assert(message.Error, Matches, ".*repository is empty.*")
	})
}

func (s *BoxerSuite) TestRepoDownloadArchiveTimeout(c *C) {
	s.testEachStrategy(c, func(s *BoxerSuite, c *C, stratName string,
		stratValue testProvisionStrategy) {

		message, box := s.receiveMessages("mclovins-container-1",
			s.makeCreateMessage(defaultMsg),
			s.makeMessage("aio:start", defaultMsg))
		c.Assert(message.Success, Equals, true)

		// Note: not a full integration test unlike the rest so that we can
		// pass a custom RetrieveTimeout
		repo := &Repo{
			Box:             box,
			Url:             "http://10.9.8.7/nitrous-io/linux/archive/stable.tar.gz",
			Scm:             ScmTargz,
			RetrieveTimeout: 1 * time.Second,
		}
		ValidateRepo(repo)

		err := repo.Retrieve(filepath.Join("tmp", "stable.tar.gz"))

		c.Assert(err, NotNil)
		c.Assert(err, Equals, RetrieveTimeoutError)
	})
}
