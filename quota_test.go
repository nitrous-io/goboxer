package goboxer

import (
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
)

type QuotaSuite struct{}

var _ = Suite(&QuotaSuite{})

func (s *QuotaSuite) SetUpTest(c *C) {
	os.MkdirAll("/lxc/disk1/test_quota", 0644)
}

func (s *QuotaSuite) TearDownTest(c *C) {
	os.RemoveAll("/lxc/disk1/test_quota")
}

func (s *QuotaSuite) TestSetQuotaLimit(c *C) {
	err := SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "0.5G")
	c.Assert(err, IsNil)
	defer RemoveQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")

	DoesContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})

	info, err := ViewQuotaLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 512*1024)
}

func (s *QuotaSuite) TestRemoveQuotaLimit(c *C) {
	SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	err := RemoveQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	c.Assert(err, IsNil)

	DoesNotContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})
}

func (s *QuotaSuite) TestChangeQuotaLimit(c *C) {
	SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	defer RemoveQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")

	info, _ := ViewQuotaLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 3072)
}

func (s *QuotaSuite) TestChangeInvalidLimit(c *C) {
	SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "3M")
	defer RemoveQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")
	// write 2M to test
	err := exec.Command("dd", "if=/dev/zero", "of=/lxc/disk1/test_quota/test_file",
		"bs=1024", "count=2048").Run()
	c.Assert(err, IsNil)
	err = SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	c.Assert(err, NotNil)

	info, _ := ViewQuotaLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 2048)
	c.Assert(info.Limit, Equals, 3072)
}
