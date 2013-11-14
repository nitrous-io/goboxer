// Assume that we have /lxc/disk2 setup as an ext4 disk with usrquota turned on
package quota

import (
	"github.com/action-io/goboxer/utils"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
)

type UsrQuotaSuite struct{}

var _ = Suite(&UsrQuotaSuite{})

func (s *UsrQuotaSuite) SetUpTest(c *C) {
	utils.RunCommand("useradd", "-u", "15999", "-r", "userquotatest")
	os.MkdirAll("/lxc/disk2/test_quota", 0644)
	utils.RunCommand("chown", "15999", "/lxc/disk2/test_quota")
}

func (s *UsrQuotaSuite) TearDownTest(c *C) {
	utils.RunCommand("userdel", "userquotatest")
	os.RemoveAll("/lxc/disk2/test_quota")
}

func (s *UsrQuotaSuite) TestSetLimit(c *C) {
	q := UsrQuota{}
	arg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "1M"}
	q.SetLimit(arg)
	defer q.RemoveLimit(arg)
	info, _ := q.ViewLimit(arg)
	c.Assert(info.Used, NotNil)
	c.Assert(info.Limit, Equals, 1024)
}

func (s *UsrQuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := UsrQuota{}
	arg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "3M"}
	q.SetLimit(arg)
	q.SetLimit(arg)
	defer q.RemoveLimit(arg)

	info, _ := q.ViewLimit(arg)
	c.Assert(info.Limit, Equals, 3*1024)
}

func (s *UsrQuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := UsrQuota{}
	arg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "1M"}
	q.SetLimit(arg)
	q.RemoveLimit(arg)
	info, _ := q.ViewLimit(arg)
	c.Assert(info.Limit, Equals, 0)
}

func (s *UsrQuotaSuite) TestChangeQuotaLimit(c *C) {
	q := UsrQuota{}
	oldArg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "1M"}
	newArg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "3M"}
	q.SetLimit(oldArg)
	defer q.RemoveLimit(newArg)
	q.SetLimit(newArg)

	info, _ := q.ViewLimit(newArg)
	c.Assert(info.Used, NotNil)
	c.Assert(info.Limit, Equals, 3072)
}

func (s *UsrQuotaSuite) TestChangeInvalidLimit(c *C) {
	q := UsrQuota{}
	oldArg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "3M"}
	newArg := QuotaArg{Id: "999", Disk: "/lxc/disk2", Limit: "1M"}
	q.SetLimit(oldArg)
	defer q.RemoveLimit(oldArg)

	// write 2M to test
	exec.Command("dd", "if=/dev/zero", "of=/lxc/disk2/test_quota/test_file",
		"bs=1024", "count=2048").Run()
	exec.Command("chown", "15999", "/lxc/disk2/test_quota/test_file").Run()
	exec.Command("sync").Run()

	err := q.SetLimit(newArg)
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit(oldArg)
	c.Assert((info.Used-2*1024) > 0, Equals, true)
	c.Assert(info.Limit, Equals, 3*1024)
}
