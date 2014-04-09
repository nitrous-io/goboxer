package goboxer

import (
	"encoding/json"
	"github.com/action-io/goboxer/utils"
	. "github.com/motain/gocheck"
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
	c.Assert(b.TotalMemory(), Equals, "576M")
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
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk2",
		Provisioner: "lxc"}
	b.boxer = &Boxer{}
	c.Assert(b.LxcPath(), Equals, "/lxc/disk2/test-123-123456")
}

func (s *BoxSuite) TestRootFS(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk1",
		MountMethod: "aufs", Provisioner: "lxc"}
	b.boxer = &Boxer{}
	c.Assert(b.RootFS(), Equals, "/lxc/disk1/test-123-123456/rootfs")
}

func (s *BoxSuite) TestContainerDeltaPath(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk1",
		MountMethod: "aufs"}
	b.boxer = &Boxer{}
	c.Assert(b.ContainerDeltaPath(), Equals, "/lxc/disk1/test-123_delta")
}

func (s *BoxSuite) TestContainerUserHomePath(c *C) {
	b := &Box{Name: "test", Id: 123, UniqueId: "123456", Disk: "/lxc/disk1", MountMethod: "aufs"}
	b.boxer = &Boxer{}
	c.Assert(b.ContainerUserHomePath(), Equals, "/lxc/disk1/test-123_delta/rootfs/home/action")
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
	c.Assert(b.String(), Contains, "Id:1")
	c.Assert(b.String(), Contains, "Name:fogell")
	c.Assert(b.String(), Contains, "UniqueId:1234")
	c.Assert(b.String(), Contains, "State:stopped")
	c.Assert(b.String(), Contains, "Ip:192.168.1.100")
	c.Assert(b.String(), Contains, "Port:10000")
	c.Assert(b.String(), Contains, "Disk:/lxc/disk1")
	c.Assert(b.String(), Contains, "MountMethod:aufs")
	c.Assert(b.String(), Contains, "BoxerId:testboxer")
	c.Assert(b.String(), Contains, "Premium:false")
}
