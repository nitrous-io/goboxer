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
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	s.conn.Do("RPUSH", "aio:available_resources", ip)
	s.conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *ResourcePoolSuite) TestAcquireAvailableResource(c *C) {
	s.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := &ResourcePool{Conn: s.conn}

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
	rs := &ResourcePool{Conn: s.conn}
	_, err := rs.Acquire()
	c.Assert(err, NotNil)
}

func (s *ResourcePoolSuite) TestReleaseValidResource(c *C) {
	s.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := &ResourcePool{Conn: s.conn}
	rs.Acquire()
	rs.Release("192.168.2.2")
	availableIp, _ := redis.String(s.conn.Do("RPOP", "aio:available_resources"))
	c.Assert(availableIp, Equals, "192.168.2.2")
}

func makeTestBox(id, state string) *Box {
	return &Box{
		Id:               id,
		State:            state,
		Name:             "test",
		Runtime:          "test",
		Template:         "test_template",
		Ip:               "192.168.254.2",
		Gateway:          "br-lxc-254",
		NetworkInterface: "192.168.254.1",
		Memory:           "256M",
		DiskSpace:        "1G",
	}

}

func (s *ResourcePoolSuite) TestFindRunningBoxes(c *C) {
	box1json, _ := ToJSON(makeTestBox("1", "running"))
	box2json, _ := ToJSON(makeTestBox("2", "stopped"))
	box3json, _ := ToJSON(makeTestBox("3", "running"))
	s.conn.Do("HSET", "aio:boxes", "test-1", box1json)
	s.conn.Do("HSET", "aio:boxes", "test-2", box2json)
	s.conn.Do("HSET", "aio:boxes", "test-3", box3json)

	rs := &ResourcePool{Conn: s.conn}
	b, _ := rs.FindRunningBoxes()

	c.Assert(len(b), Equals, 2)
	c.Assert(b[0].Id, Equals, "3")
	c.Assert(b[0].State, Equals, "running")
	c.Assert(b[1].Id, Equals, "1")
	c.Assert(b[1].State, Equals, "running")
}
