// Assume that we have aio_boxhost with boxes thin pool setup
package quota

import (
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
)

type LvmQuotaSuite struct{}

var _ = Suite(&LvmQuotaSuite{})

func (s *LvmQuotaSuite) SetUpTest(c *C) {
	RunCommand("umount", "/var/lib/lxc/test_quota")
	RunCommand("lvremove", "-f", "aio_boxhost/test-1")
	RunCommand("lvcreate", "-T", "aio_boxhost/boxes", "-V", "10M", "-n",
		"test-1")
	os.RemoveAll("/var/lib/lxc/test_quota")
	os.MkdirAll("/var/lib/lxc/test_quota", 0644)
	RunCommand("mkfs.ext4", "/dev/aio_boxhost/test-1")
	RunCommand("mount", "-t", "ext4", "/dev/aio_boxhost/test-1",
		"/var/lib/lxc/test_quota")
}

func (s *LvmQuotaSuite) TearDownTest(c *C) {
	RunCommand("umount", "/var/lib/lxc/test_quota")
	RunCommand("lvremove", "-f", "aio_boxhost/test-1")
	os.RemoveAll("/var/lib/lxc/test_quota")
}

func (s *LvmQuotaSuite) TestSetLimit(c *C) {
	q := LvmQuota{}
	arg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota",
		Limit: "15M"}
	q.SetLimit(arg)
	defer q.RemoveLimit(arg)
	info, _ := q.ViewLimit(arg)
	c.Assert(info.Used, NotNil)
	// size of lvm is approximate
	c.Assert((info.Limit-15*1024) >= 0, Equals, true)
}

func (s *LvmQuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := LvmQuota{}
	arg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota",
		Limit: "15M"}
	q.SetLimit(arg)
	q.SetLimit(arg)
	defer q.RemoveLimit(arg)

	info, _ := q.ViewLimit(arg)
	c.Assert((info.Limit-15*1024) >= 0, Equals, true)
}

func (s *LvmQuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := LvmQuota{}
	arg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota"}
	q.SetLimit(arg)
	err := q.RemoveLimit(arg)
	c.Assert(err, IsNil)
}

func (s *LvmQuotaSuite) TestChangeQuotaLimit(c *C) {
	q := LvmQuota{}
	oldArg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota", Limit: "15M"}
	newArg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota", Limit: "30M"}
	q.SetLimit(oldArg)
	defer q.RemoveLimit(newArg)
	q.SetLimit(newArg)

	info, _ := q.ViewLimit(newArg)
	c.Assert(info.Used, NotNil)
	c.Assert((32*1024-info.Limit) >= 0, Equals, true)
}

func (s *LvmQuotaSuite) TestChangeInvalidLimit(c *C) {
	q := LvmQuota{}
	oldArg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota", Limit: "30M"}
	newArg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota", Limit: "15M"}
	q.SetLimit(oldArg)
	defer q.RemoveLimit(oldArg)

	// write 16M to test
	exec.Command("dd", "if=/dev/zero", "of=/var/lib/lxc/test_quota/test_file",
		"bs=1024", "count=16384").Run()

	err := q.SetLimit(newArg)
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit(oldArg)
	c.Assert((info.Used-16*1024) > 0, Equals, true)
	c.Assert((32*1024-info.Limit) >= 0, Equals, true)
	c.Assert((16*1024-info.Limit) >= 0, Equals, false)
}
