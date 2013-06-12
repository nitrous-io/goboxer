package goboxer

import (
	. "launchpad.net/gocheck"
	"os"
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
	err := SetQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1", "1M")
	c.Assert(err, IsNil)
	defer RemoveQuotaLimit("123", "test-123", "/lxc/disk1/test_quota", "/lxc/disk1")

	DoesContain(c, map[string][]string{
		"/etc/projid":   {"test-123:123"},
		"/etc/projects": {"123:/lxc/disk1/test_quota"},
	})

	info, err := ViewQuotaLimit("123", "/lxc/disk1")
	c.Assert(info.Used, Equals, 0)
	c.Assert(info.Limit, Equals, 1024)
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
