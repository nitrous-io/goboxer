// Assume that we have aio_boxhost with boxes thin pool setup
package quota

import (
	. "launchpad.net/gocheck"
	"math"
	"os"
)

type LvmQuotaSuite struct{}

var _ = Suite(&LvmQuotaSuite{})

func (s *LvmQuotaSuite) SetUpSuite(c *C) {
	// skip lvm tests as we wont using it now
	c.Skip("skip lvmquota tests as we do not support it now")
}

func (s *LvmQuotaSuite) SetUpTest(c *C) {
	run("umount", "/var/lib/lxc/test_quota")
	run("lvremove", "-f", "aio_boxhost/test-1")
	run("lvcreate", "-T", "aio_boxhost/boxes", "-V", "10M", "-n",
		"test-1")
	os.RemoveAll("/var/lib/lxc/test_quota")
	os.MkdirAll("/var/lib/lxc/test_quota", 0644)
	run("mkfs.ext4", "/dev/aio_boxhost/test-1")
	run("mount", "-t", "ext4", "/dev/aio_boxhost/test-1",
		"/var/lib/lxc/test_quota")
}

func (s *LvmQuotaSuite) TearDownTest(c *C) {
	run("umount", "/var/lib/lxc/test_quota")
	run("lvremove", "-f", "aio_boxhost/test-1")
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
	c.Assert(math.Abs(float64(info.Limit-15*1024)) <= 1024, Equals, true)
}

func (s *LvmQuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := LvmQuota{}
	arg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota",
		Limit: "15M"}
	q.SetLimit(arg)
	q.SetLimit(arg)
	defer q.RemoveLimit(arg)

	info, _ := q.ViewLimit(arg)
	//c.Assert((info.Limit-15*1024) >= 0, Equals, true)
	c.Assert(math.Abs(float64(info.Limit-15*1024)) <= 1024, Equals, true)
}

func (s *LvmQuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := LvmQuota{}
	arg := QuotaArg{Id: "1", Name: "test-1", Path: "/var/lib/lxc/test_quota", Limit: "15M"}
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
