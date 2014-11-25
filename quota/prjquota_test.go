package quota

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	. "github.com/motain/gocheck"
)

type PrjQuotaSuite struct{}

var _ = Suite(&PrjQuotaSuite{})

func (s *PrjQuotaSuite) SetUpSuite(c *C) {
	c.Skip("skip prjquota tests as we do not support it now")
}

func (s *PrjQuotaSuite) SetUpTest(c *C) {
	os.RemoveAll("/lxc/disk1/test_quota")
	os.MkdirAll("/lxc/disk1/test_quota", 0644)
	run("mv", "/etc/projid", "/etc/projid.bk")
	run("mv", "/etc/projects", "/etc/projects.bk")
}

func (s *PrjQuotaSuite) TearDownTest(c *C) {
	os.RemoveAll("/lxc/disk1/test_quota")
	run("mv", "/etc/projid.bk", "/etc/projid")
	run("mv", "/etc/projects.bk", "/etc/projects")
}

func (s *PrjQuotaSuite) TestSetQuotaLimit(c *C) {
	q := PrjQuota{}
	arg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota", Disk: "/lxc/disk1", Limit: "0.5G"}
	err := q.SetLimit(arg)
	c.Assert(err, IsNil)
	defer q.RemoveLimit(arg)

	DoesContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})

	info, _ := q.ViewLimit(arg)
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 512*1024)
}

func (s *PrjQuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := PrjQuota{}
	arg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota",
		Disk: "/lxc/disk1", Limit: "3M"}
	q.SetLimit(arg)
	q.SetLimit(arg)
	defer q.RemoveLimit(arg)

	// it doesnt duplicate the contents of projid and projects as well as
	// xfs_quota
	projidContent, _ := ioutil.ReadFile("/etc/projid")
	projectsContent, _ := ioutil.ReadFile("/etc/projects")

	c.Assert(strings.Count(string(projidContent), "test-123"), Equals, 1)
	c.Assert(strings.Count(string(projectsContent), "test_quota"), Equals, 1)

	info, _ := q.ViewLimit(arg)
	c.Assert(info.Limit, Equals, 3*1024)
}

func (s *PrjQuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := PrjQuota{}
	arg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota",
		Disk: "/lxc/disk1", Limit: "1M"}
	q.SetLimit(arg)
	err := q.RemoveLimit(arg)
	c.Assert(err, IsNil)

	DoesNotContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})
}

func (s *PrjQuotaSuite) TestChangeQuotaLimit(c *C) {
	q := PrjQuota{}
	oldArg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota", Disk: "/lxc/disk1", Limit: "1M"}
	newArg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota", Disk: "/lxc/disk1", Limit: "3M"}
	q.SetLimit(oldArg)
	defer q.RemoveLimit(newArg)
	q.SetLimit(newArg)

	info, _ := q.ViewLimit(newArg)
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 3072)
}

func (s *PrjQuotaSuite) TestChangeInvalidLimit(c *C) {
	q := PrjQuota{}
	oldArg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota", Disk: "/lxc/disk1", Limit: "3M"}
	q.SetLimit(oldArg)
	defer q.RemoveLimit(oldArg)
	// write 2M to test
	exec.Command("dd", "if=/dev/zero", "of=/lxc/disk1/test_quota/test_file",
		"bs=1024", "count=2048").Run()

	newArg := QuotaArg{Id: "123", Name: "test-123", Path: "/lxc/disk1/test_quota", Disk: "/lxc/disk1", Limit: "1M"}
	err := q.SetLimit(newArg)
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit(newArg)
	c.Assert(info.Used, Equals, 2048)
	c.Assert(info.Limit, Equals, 3072)
}
