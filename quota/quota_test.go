package quota

import (
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
)

type QuotaSuite struct{}

var _ = Suite(&QuotaSuite{})

const DISK = "/var/lib/docker"
const TEST_DIR = DISK + "/test_quota"

// /var/lib/docker is the test ext4 disk
func (s *QuotaSuite) SetUpTest(c *C) {
	RunCommand("useradd", "-u", "15999", "-r", "userquotatest")
	os.MkdirAll(TEST_DIR, 0644)
	RunCommand("chown", "15999", TEST_DIR)
}

func (s *QuotaSuite) TearDownTest(c *C) {
	//RunCommand("userdel", "userquotatest")
	//os.RemoveAll(TEST_DIR)
}

func (s *QuotaSuite) TestSetLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", DISK, "1M")
	defer q.RemoveLimit("999", DISK)

	info, _ := q.ViewLimit("999", DISK)
	c.Assert(info.Limit, Equals, 1024)
}

func (s *QuotaSuite) TestSetLimitMultipleTimes(c *C) {
	q := Quota{}
	q.SetLimit("999", DISK, "3M")
	q.SetLimit("999", DISK, "3M")
	defer q.RemoveLimit("999", DISK)

	info, _ := q.ViewLimit("999", DISK)
	c.Assert(info.Limit, Equals, 3*1024)
}

func (s *QuotaSuite) TestRemoveQuotaLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", DISK, "1M")
	q.RemoveLimit("999", DISK)
	info, _ := q.ViewLimit("999", DISK)
	c.Assert(info.Limit, Equals, 0)
}

func (s *QuotaSuite) TestChangeQuotaLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", DISK, "1M")
	defer q.RemoveLimit("999", DISK)
	q.SetLimit("999", DISK, "3M")

	info, _ := q.ViewLimit("999", "/lxc/disk1")
	c.Assert(info.Limit, Equals, 3072)
}

func (s *QuotaSuite) TestChangeInvalidLimit(c *C) {
	q := Quota{}
	q.SetLimit("999", DISK, "3M")
	defer q.RemoveLimit("999", DISK)
	q.ViewLimit("999", DISK)

	// write 2M to test
	exec.Command("dd", "if=/dev/zero", "of="+TEST_DIR+"/test_file",
		"bs=1024", "count=2048").Run()
	exec.Command("chown", "15999", TEST_DIR+"/test_file").Run()

	q.ViewLimit("999", DISK)

	err := q.SetLimit("999", DISK, "1M")
	c.Assert(err, NotNil)

	info, _ := q.ViewLimit("999", DISK)
	c.Assert(info.Used >= 2*1024, Equals, true)
	c.Assert(info.Limit, Equals, 3*1024)
}
