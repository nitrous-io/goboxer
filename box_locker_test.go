package goboxer

import (
	. "github.com/motain/gocheck"
)

type BoxLockerSuite struct{}

var _ = Suite(&BoxLockerSuite{})

func (s *BoxLockerSuite) TestLockUnlock(c *C) {
	locker := BoxLocker{}
	locker.Lock(1)
	c.Assert(locker[1].locked, Equals, true)
	locker.Unlock(1)
	c.Assert(locker[1].locked, Equals, false)
}
