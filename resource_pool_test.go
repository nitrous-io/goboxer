package goboxer

import (
	"time"

	"github.com/action-io/goboxer/utils"
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
)

type ResourcePoolSuite struct {
	pool *redis.Pool
}

var _ = Suite(&ResourcePoolSuite{})

func (s *ResourcePoolSuite) SetUpTest(c *C) {
	s.pool = &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", ":6379")
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	conn := s.pool.Get()
	defer conn.Close()
	conn.Do("FLUSHDB")
}

func (s *ResourcePoolSuite) TearDownTest(c *C) {
	defer s.pool.Close()
	conn := s.pool.Get()
	defer conn.Close()
	conn.Do("FLUSHDB")
}

func (s *ResourcePoolSuite) insertResource(ip, gateway, networkInterface, port string) {
	conn := s.pool.Get()
	defer conn.Close()
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	conn.Do("RPUSH", "aio:available_resources", ip)
	conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *ResourcePoolSuite) TestAcquireAvailableResource(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	s.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := &ResourcePool{Pool: s.pool}

	r, err := rs.Acquire()
	c.Assert(err, IsNil, Commentf("failed to acquire resource"))

	usedResource, err := redis.String(conn.Do("RPOP", "aio:used_resources"))
	c.Assert(usedResource, Equals, "192.168.2.2")
	c.Assert(r.Ip, Equals, "192.168.2.2")
	c.Assert(r.Gateway, Equals, "192.168.2.1")
	c.Assert(r.NetworkInterface, Equals, "br-lxc-254")
	c.Assert(r.Port, Equals, "10002")
}

func (s *ResourcePoolSuite) TestAcquireNonAvailableResource(c *C) {
	rs := &ResourcePool{Pool: s.pool}
	_, err := rs.Acquire()
	c.Assert(err, NotNil)
}

func (s *ResourcePoolSuite) TestReleaseValidResource(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	s.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := &ResourcePool{Pool: s.pool}
	rs.Acquire()
	rs.Release("192.168.2.2")
	availableIp, _ := redis.String(conn.Do("RPOP", "aio:available_resources"))
	c.Assert(availableIp, Equals, "192.168.2.2")
}

func makeTestBox(id int, state string) *Box {
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
	conn := s.pool.Get()
	defer conn.Close()
	box1json, _ := utils.ToJSON(makeTestBox(1, "running"))
	box2json, _ := utils.ToJSON(makeTestBox(2, "stopped"))
	box3json, _ := utils.ToJSON(makeTestBox(3, "running"))
	conn.Do("HSET", "aio:boxes", "test-1", box1json)
	conn.Do("HSET", "aio:boxes", "test-2", box2json)
	conn.Do("HSET", "aio:boxes", "test-3", box3json)

	rs := &ResourcePool{Pool: s.pool}
	b, _ := rs.FindRunningBoxes()

	c.Assert(len(b), Equals, 2)
	c.Assert(b[0].Id, Equals, 3)
	c.Assert(b[0].State, Equals, "running")
	c.Assert(b[1].Id, Equals, 1)
	c.Assert(b[1].State, Equals, "running")
}

func (s *ResourcePoolSuite) TestCountAllBoxes(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	box1json, _ := utils.ToJSON(makeTestBox(1, "running"))
	box2json, _ := utils.ToJSON(makeTestBox(2, "stopped"))
	conn.Do("HSET", "aio:boxes", "test-1", box1json)
	conn.Do("HSET", "aio:boxes", "test-2", box2json)

	rs := &ResourcePool{Pool: s.pool}
	count, _ := rs.CountAllBoxes()

	c.Assert(count, Equals, 2)
}

func (s *ResourcePoolSuite) TestCountBoxesByState(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	box1json, _ := utils.ToJSON(makeTestBox(1, "running"))
	box2json, _ := utils.ToJSON(makeTestBox(2, "stopped"))
	box3json, _ := utils.ToJSON(makeTestBox(3, "running"))
	conn.Do("HSET", "aio:boxes", "test-1", box1json)
	conn.Do("HSET", "aio:boxes", "test-2", box2json)
	conn.Do("HSET", "aio:boxes", "test-3", box3json)

	rs := &ResourcePool{Pool: s.pool}
	runningCount, _ := rs.CountBoxesByState("running")
	stoppedCount, _ := rs.CountBoxesByState("stopped")

	c.Assert(runningCount, Equals, 2)
	c.Assert(stoppedCount, Equals, 1)
}

func (s *ResourcePoolSuite) TestCountPremiumBoxes(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	box1 := makeTestBox(1, "running")
	box1.Premium = true
	box1json, _ := utils.ToJSON(box1)
	box2json, _ := utils.ToJSON(makeTestBox(2, "running"))
	conn.Do("HSET", "aio:boxes", "test-1", box1json)
	conn.Do("HSET", "aio:boxes", "test-2", box2json)

	rs := &ResourcePool{Pool: s.pool}
	count, _ := rs.CountPremiumBoxes()
	c.Assert(count, Equals, 1)
}
