package quota

import (
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
)

type UserQuotaSuite struct{}

var _ = Suite(&UserQuotaSuite{})

func (s *UserQuotaSuite) SetUpTest(c *C) {
	RunCommand("useradd", "-u", "15999", "-r", "userquotatest")
	os.MkdirAll("/lxc/disk1/test_quota", 0644)
	RunCommand("chown", "15999", "/lxc/disk1/test_quota")
}

func (s *UserQuotaSuite) TearDownTest(c *C) {
	RunCommand("userdel", "userquotatest")
	os.RemoveAll("/lxc/disk1/test_quota")
}

func (s *UserQuotaSuite) TestSetLimit(c *C) {
	q := UserQuota{}
	q.SetLimit("999", "", "", "/lxc/disk1", "1M")
	defer q.RemoveLimit("999", "", "", "/lxc/disk1")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 1024)
}

func (s *UserQuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := UserQuota{}
	q.SetLimit("999", "", "", "/lxc/disk1", "3M")
	q.SetLimit("999", "", "", "/lxc/disk1", "3M")
	defer q.RemoveLimit("999", "", "", "/lxc/disk1")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Limit, Equals, 3*1024)
}

func (s *UserQuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := UserQuota{}
	q.SetLimit("999", "", "", "/lxc/disk1", "1M")
	q.RemoveLimit("999", "", "", "/lxc/disk1")
	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Limit, Equals, 0)
}

func (s *UserQuotaSuite) TestChangeQuotaLimit(c *C) {
	q := UserQuota{}
	q.SetLimit("999", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	defer q.RemoveLimit("999", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	q.SetLimit("999", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 3072)
}

func (s *UserQuotaSuite) TestChangeInvalidLimit(c *C) {
	q := UserQuota{}
	q.SetLimit("999", "", "", "/lxc/disk1", "3M")
	defer q.RemoveLimit("999", "", "", "/lxc/disk1")
	q.ViewLimit("999", "/lxc/disk1")

	// write 2M to test
	exec.Command("dd", "if=/dev/zero", "of=/lxc/disk1/test_quota/test_file",
		"bs=1024", "count=2048").Run()
	exec.Command("chown", "15999", "/lxc/disk1/test_quota/test_file").Run()

	q.ViewLimit("999", "/lxc/disk1")

	err := q.SetLimit("999", "", "", "/lxc/disk1", "1M")
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Used, Equals, 2*1024)
	c.Assert(info.Limit, Equals, 3*1024)
}
