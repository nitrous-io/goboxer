package lxc

import (
	"fmt"
	. "github.com/action-io/goboxer/checks"
	lxcCmd "github.com/action-io/goboxer/lxc"
	pr "github.com/action-io/goboxer/provisioner"
	"github.com/action-io/goboxer/s3"
	. "github.com/motain/gocheck"
	"io/ioutil"
)

var defaultConfig = pr.Config{
	Runtime:               "test",
	Premium:               false,
	ContainerName:         "test-lxc-1",
	ContainerId:           1,
	Uid:                   "15001",
	Template:              "test_container_template",
	Ip:                    "192.168.254.254",
	Gateway:               "192.168.254.1",
	NetworkInterface:      "br-lxc-254",
	MacAddress:            "02:00:c0:a8:fe:fe",
	CpuQuota:              1.0,
	CpuPeriodMicroseconds: 1000000,
	CpuQuotaMicroseconds:  1000000,
}

type LxcProvisionerSuite struct {
	provisioner *Provisioner
}

var _ = Suite(&LxcProvisionerSuite{})

func (s *LxcProvisionerSuite) SetUpTest(c *C) {
	p, _ := Init("/lxc/disk2")
	s.provisioner, _ = p.(*Provisioner)
}

func (s *LxcProvisionerSuite) TestCreate(c *C) {
	config := defaultConfig
	id, root, err := s.provisioner.Create(config)
	config.Id = id
	defer s.provisioner.Destroy(config)

	c.Assert(err, IsNil)
	c.Assert(root, Equals, fmt.Sprintf("/lxc/disk2/%s-%s/rootfs", "test-lxc-1", id))
}

func (s *LxcProvisionerSuite) TestCreateFromSnapshot(c *C) {
	config := defaultConfig
	config.SnapshotUrl = "s3://ap-southeast-1/staging-apse1.action.io/testboxer-1234.tar.gz"
	id, root, err := s.provisioner.CreateFromSnapshot(config, s3.AuthTest{})
	config.Id = id
	defer s.provisioner.Destroy(config)

	c.Assert(err, IsNil)
	c.Assert(root, Equals, fmt.Sprintf("/lxc/disk2/%s-%s/rootfs", "test-lxc-1", id))
	// from s3 stub data
	c.Assert(fmt.Sprintf("%s/home/action/test.txt", root), Exists)
}

func (s *LxcProvisionerSuite) TestStartStop(c *C) {
	config := defaultConfig
	id, _, _ := s.provisioner.Create(config)
	config.Id = id
	// start the container
	id, _, _ = s.provisioner.Start(config, pr.NoopCallback)
	config.Id = id
	status, _ := lxcCmd.Status(config.ContainerName + "-" + config.Id)
	c.Assert(status, Equals, lxcCmd.RUNNING)
	// stop the container
	id, _ = s.provisioner.Stop(config)
	status, _ = lxcCmd.Status(config.ContainerName + "-" + config.Id)
	c.Assert(status, Equals, "")
	config.Id = id
	// destroy the container
	err := s.provisioner.Destroy(config)
	c.Assert(err, IsNil)
}

func (s *LxcProvisionerSuite) TestDestroy(c *C) {
	config := defaultConfig
	id, root, _ := s.provisioner.Create(config)
	config.Id = id
	c.Assert(root, Exists)

	s.provisioner.Destroy(config)
	c.Assert(root, Not(Exists))
}

func (s *LxcProvisionerSuite) TestChangeConfig(c *C) {
	config := defaultConfig
	id, root, _ := s.provisioner.Create(config)
	config.Id = id
	c.Assert(root, Exists)
	// start the container
	config.Id, _, _ = s.provisioner.Start(config, pr.NoopCallback)
	defer func() {
		id, _ = s.provisioner.Stop(config)
		config.Id = id
		s.provisioner.Destroy(config)
	}()

	// change config
	config.CpuQuota = 2.0
	config.CpuQuotaMicroseconds = 2000000
	err := s.provisioner.ChangeConfig(config)
	c.Assert(err, IsNil)

	lxcConfig := s.provisioner.GetConfig(config.ContainerName, id)
	content, _ := ioutil.ReadFile(lxcConfig)
	c.Assert(string(content), Contains, config.ContainerName)
}

func (s *LxcProvisionerSuite) TestGetRoot(c *C) {
	config := defaultConfig
	id, root, _ := s.provisioner.Create(config)
	config.Id = id
	c.Assert(root, Exists)
	defer s.provisioner.Destroy(config)

	rootfs := s.provisioner.GetRoot(config.ContainerName, id)
	c.Assert(root, Equals, rootfs)
}

func (s *LxcProvisionerSuite) TestGetConfig(c *C) {
	config := defaultConfig
	id, root, _ := s.provisioner.Create(config)
	config.Id = id
	c.Assert(root, Exists)
	defer s.provisioner.Destroy(config)

	lxcConfig := s.provisioner.GetConfig(config.ContainerName, id)
	content, _ := ioutil.ReadFile(lxcConfig)
	c.Assert(string(content), Contains, config.ContainerName)
}
