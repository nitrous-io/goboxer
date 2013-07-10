package quota

import (
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
)

type QuotaSuite struct{}

var _ = Suite(&QuotaSuite{})

func (s *QuotaSuite) SetUpTest(c *C) {
	RunCommand("useradd", "-u", "15999", "-r", "userquotatest")
	os.MkdirAll("/lxc/disk1/test_quota", 0644)
	RunCommand("chown", "15999", "/lxc/disk1/test_quota")
}

func (s *QuotaSuite) TearDownTest(c *C) {
	RunCommand("userdel", "userquotatest")
	os.RemoveAll("/lxc/disk1/test_quota")
}

func (s *QuotaSuite) TestSetLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", "/lxc/disk1", "1M")
	defer q.RemoveLimit("999", "/lxc/disk1")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 1024)
}

func (s *QuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := Quota{}
	q.SetLimit("999", "/lxc/disk1", "3M")
	q.SetLimit("999", "/lxc/disk1", "3M")
	defer q.RemoveLimit("999", "/lxc/disk1")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Limit, Equals, 3*1024)
}

func (s *QuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", "/lxc/disk1", "1M")
	q.RemoveLimit("999", "/lxc/disk1")
	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Limit, Equals, 0)
}

func (s *QuotaSuite) TestChangeQuotaLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", "/lxc/disk1", "1M")
	defer q.RemoveLimit("999", "/lxc/disk1")
	q.SetLimit("999", "/lxc/disk1", "3M")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 3072)
}

func (s *QuotaSuite) TestChangeInvalidLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", "/lxc/disk1", "3M")
	defer q.RemoveLimit("999", "/lxc/disk1")
	q.ViewLimit("999", "/lxc/disk1")

	// write 2M to test
	exec.Command("dd", "if=/dev/zero", "of=/lxc/disk1/test_quota/test_file",
		"bs=1024", "count=2048").Run()
	exec.Command("chown", "15999", "/lxc/disk1/test_quota/test_file").Run()

	q.ViewLimit("999", "/lxc/disk1")

	err := q.SetLimit("999", "/lxc/disk1", "1M")
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Used, Equals, 2*1024)
	c.Assert(info.Limit, Equals, 3*1024)
}
