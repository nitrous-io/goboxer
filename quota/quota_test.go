package quota

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"strings"
)

type QuotaSuite struct{}

var _ = Suite(&QuotaSuite{})

func (s *QuotaSuite) SetUpTest(c *C) {
	os.MkdirAll("/lxc/disk1/test_quota", 0644)
	RunCommand("mv", "/etc/projid", "/etc/projid.bk")
	RunCommand("mv", "/etc/projects", "/etc/projects.bk")
}

func (s *QuotaSuite) TearDownTest(c *C) {
	os.RemoveAll("/lxc/disk1/test_quota")
	RunCommand("mv", "/etc/projid.bk", "/etc/projid")
	RunCommand("mv", "/etc/projects.bk", "/etc/projects")
}

func (s *QuotaSuite) TestSetQuotaLimit(c *C) {
	q := Quota{}
	err := q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "0.5G")
	c.Assert(err, IsNil)
	defer q.RemoveLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")

	DoesContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})

	info, _ := q.ViewLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 512*1024)
}

func (s *QuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := Quota{}
	q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")
	q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")
	defer q.RemoveLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")

	// it doesnt duplicate the contents of projid and projects as well as
	// xfs_quota
	projidContent, _ := ioutil.ReadFile("/etc/projid")
	projectsContent, _ := ioutil.ReadFile("/etc/projects")

	c.Assert(strings.Count(string(projidContent), "test-123"), Equals, 1)
	c.Assert(strings.Count(string(projectsContent), "test_quota"), Equals, 1)

	info, _ := q.ViewLimit("123", "/lxc/disk1")
	c.Assert(info.Limit, Equals, 3*1024)
}

func (s *QuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := Quota{}
	q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	err := q.RemoveLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	c.Assert(err, IsNil)

	DoesNotContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})
}

func (s *QuotaSuite) TestChangeQuotaLimit(c *C) {
	q := Quota{}
	q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	defer q.RemoveLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")

	info, _ := q.ViewLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 3072)
}

func (s *QuotaSuite) TestChangeInvalidLimit(c *C) {
	q := Quota{}
	q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")
	defer q.RemoveLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	// write 2M to test
	err := exec.Command("dd", "if=/dev/zero", "of=/lxc/disk1/test_quota/test_file",
		"bs=1024", "count=2048").Run()
	c.Assert(err, IsNil)
	err = q.SetLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 2048)
	c.Assert(info.Limit, Equals, 3072)
}
