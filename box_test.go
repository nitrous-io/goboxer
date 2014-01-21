package goboxer

import (
	"encoding/json"
	"github.com/action-io/goboxer/utils"
	. "launchpad.net/gocheck"
	"strings"
)

type BoxSuite struct{}

var _ = Suite(&BoxSuite{})

func (s *BoxSuite) TestPrepare(c *C) {
	b := &Box{}
	b.Prepare()
	c.Assert(b.Swappiness, Equals, 60) // FIXME How to stub constants in Golang?
	c.Assert(b.CpuShares, Equals, 512)
}

func (s *BoxSuite) TestTotalMemory(c *C) {
	b := &Box{Memory: "384M"}
	c.Assert(b.TotalMemory(), Equals, "768M")
}

func (s *BoxSuite) TestMarshal(c *C) {
	testData := struct {
		Id   string `json:"id"`
		Port int    `json:"port"`
	}{"1", 10000}
	b, _ := utils.ToJSON(testData)
	box := &Box{}
	json.Unmarshal([]byte(b), box)
	c.Assert(box.Port, Equals, 10000)
}

type testMountMethod struct {
	name     string
	expected string
}

func (s *BoxSuite) TestMountMethod(c *C) {
	b := &Box{}
	c.Assert(b.getMountMethod(), Equals, "overlayfs")

	b.MountMethod = "aufs"
	c.Assert(b.getMountMethod(), Equals, "aufs")

	b.MountMethod = ""
	b.Aufs = true
	c.Assert(b.getMountMethod(), Equals, "aufs")

	b.MountMethod = "lvm"
	c.Assert(b.getMountMethod(), Equals, "lvm")
}

func (s *BoxSuite) TestQuotaMethod(c *C) {
	b := &Box{}
	c.Assert(b.getQuotaMethod(), Equals, "prjquota")

	b.QuotaMethod = "usrquota"
	c.Assert(b.getQuotaMethod(), Equals, "usrquota")
}

func (s *BoxSuite) TestBoxAuthIsValid(c *C) {
	var b BoxAuth
	b = BoxAuth{
		Username: "hello",
		Password: "world",
	}
	c.Assert(b.IsValid(), Equals, true)

	b = BoxAuth{
		Username: "hello",
	}
	c.Assert(b.IsValid(), Equals, false)

	b = BoxAuth{
		Password: "hello",
	}
	c.Assert(b.IsValid(), Equals, false)

	b = BoxAuth{}
	c.Assert(b.IsValid(), Equals, true)
}

func (s *BoxSuite) TestContainerName(c *C) {
	b := &Box{Name: "test", Id: 123}
	c.Assert(b.ContainerName(), Equals, "test-123")
}

func (s *BoxSuite) TestMountName(c *C) {
	b := &Box{Name: "test", Id: 123}
	c.Assert(b.MountName(), Equals, "test-123")
	b.UniqueId = "123456"
	c.Assert(b.MountName(), Equals, "test-123-123456")
}

func (s *BoxSuite) TestLxcPath(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456"}
	b.boxer = &Boxer{Root: "/var/lib/lxc"}
	c.Assert(b.LxcPath(), Equals, "/var/lib/lxc/test-123-123456")
}

func (s *BoxSuite) TestContainerFS(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk1",
		MountMethod: "aufs"}
	b.boxer = &Boxer{Root: "/var/lib/lxc"}
	c.Assert(b.ContainerFS(), Equals, "/lxc/disk1/test-123-123456")

	b.MountMethod = "lvm"
	c.Assert(b.ContainerFS(), Equals, "/var/lib/lxc/test-123-123456")
}

func (s *BoxSuite) TestRootFS(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk1",
		MountMethod: "aufs"}
	b.boxer = &Boxer{Root: "/var/lib/lxc"}
	c.Assert(b.RootFS(), Equals, "/var/lib/lxc/test-123-123456/rootfs")

	b.MountMethod = "lvm"
	c.Assert(b.RootFS(), Equals, "/var/lib/lxc/test-123-123456/rootfs")
}

func (s *BoxSuite) TestContainerDeltaPath(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk1",
		MountMethod: "aufs"}
	b.boxer = &Boxer{Root: "/var/lib/lxc"}
	c.Assert(b.ContainerDeltaPath(), Equals, "/lxc/disk1/test-123_delta")

	b.MountMethod = "lvm"
	c.Assert(b.ContainerDeltaPath(), Equals, "/var/lib/lxc/test-123-123456_delta")
}

func (s *BoxSuite) TestCpuSetString(c *C) {
	b := &Box{CpuSet: []int{0}}
	c.Assert(b.CpuSetStr(), Equals, "0")
	b = &Box{CpuSet: []int{1, 2, 3}}
	c.Assert(b.CpuSetStr(), Equals, "1,2,3")
}

func (s *BoxSuite) TestCpuQuotaMicroseconds(c *C) {
	b := &Box{CpuQuota: 0.5}
	c.Assert(b.CpuQuotaMicroseconds(), Equals, 500000)
	b = &Box{CpuQuota: 1}
	c.Assert(b.CpuQuotaMicroseconds(), Equals, 1000000)
	b = &Box{CpuQuota: 0}
	c.Assert(b.CpuQuotaMicroseconds(), Equals, 0)
}

func (s *BoxSuite) TestStringer(c *C) {
	b := &Box{Id: 1, Name: "fogell", UniqueId: "1234", State: "stopped", Ip: "192.168.1.100", Port: 10000, Disk: "/lxc/disk1", MountMethod: "aufs", BoxerId: "testboxer", Premium: false}
	c.Assert(strings.Contains(b.String(), "Id:1"), Equals, true)
	c.Assert(strings.Contains(b.String(), "Name:fogell"), Equals, true)
	c.Assert(strings.Contains(b.String(), "UniqueId:1234"), Equals, true)
	c.Assert(strings.Contains(b.String(), "State:stopped"), Equals, true)
	c.Assert(strings.Contains(b.String(), "Ip:192.168.1.100"), Equals, true)
	c.Assert(strings.Contains(b.String(), "Port:10000"), Equals, true)
	c.Assert(strings.Contains(b.String(), "Disk:/lxc/disk1"), Equals, true)
	c.Assert(strings.Contains(b.String(), "MountMethod:aufs"), Equals, true)
	c.Assert(strings.Contains(b.String(), "BoxerId:testboxer"), Equals, true)
	c.Assert(strings.Contains(b.String(), "Premium:false"), Equals, true)
}
