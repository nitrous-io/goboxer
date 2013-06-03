package goboxer

import (
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
)

type ResourcePoolSuite struct {
	conn redis.Conn
}

var _ = Suite(&ResourcePoolSuite{})

func (s *ResourcePoolSuite) SetUpTest(c *C) {
	conn, err := redis.Dial("tcp", ":65000")
	c.Assert(err, IsNil, Commentf("failed to establish redis connection"))
	conn.Do("Select", 13)
	conn.Do("FLUSHDB")
	s.conn = conn
}

func (s *ResourcePoolSuite) TearDownTest(c *C) {
	s.conn.Do("FLUSHDB")
	s.conn.Close()
}

func (s *ResourcePoolSuite) insertResource(ip, gateway, networkInterface, port string) {
	r := Resource{Ip: ip, Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	s.conn.Do("RPUSH", "aio:available_resources", ip)
	s.conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *ResourcePoolSuite) TestAcquireAvailableResource(c *C) {
	s.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := ResourcePool{Conn: s.conn}

	r, err := rs.Acquire()
	c.Assert(err, IsNil, Commentf("failed to acquire resource"))

	usedResource, err := redis.String(s.conn.Do("RPOP", "aio:used_resources"))
	c.Assert(usedResource, Equals, "192.168.2.2")
	c.Assert(r.Ip, Equals, "192.168.2.2")
	c.Assert(r.Gateway, Equals, "192.168.2.1")
	c.Assert(r.NetworkInterface, Equals, "br-lxc-254")
	c.Assert(r.Port, Equals, "10002")
}

func (s *ResourcePoolSuite) TestAcquireNonAvailableResource(c *C) {
	rs := ResourcePool{Conn: s.conn}
	_, err := rs.Acquire()
	c.Assert(err, NotNil)
}

func (s *ResourcePoolSuite) TestReleaseValidResource(c *C) {
	s.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := ResourcePool{Conn: s.conn}
	r, err := rs.Acquire()
	c.Assert(err, IsNil, Commentf("failed to acquire resource"))
	err = rs.Release(r)
	c.Assert(err, IsNil, Commentf("failed to release resource"))
	availableResource, err := redis.String(s.conn.Do("RPOP",
		"aio:available_resources"))
	c.Assert(availableResource, Equals, "192.168.2.2")
}
