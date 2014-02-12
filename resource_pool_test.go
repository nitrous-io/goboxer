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

func (s *ResourcePoolSuite) TestUpdateState(c *C) {
	rp := &ResourcePool{Pool: s.pool}
	conn := s.pool.Get()
	defer conn.Close()
	box := makeTestBox(1, "running")

	err := rp.UpdateState(box, "running")
	c.Check(err, IsNil)

	boxJSON, err := redis.Bytes(conn.Do("HGET", "aio:boxes", box.ContainerName()))
	c.Check(err, IsNil)
	c.Assert(string(boxJSON), Contains, `"id":1`)
	exists, _ := redis.Bool(conn.Do("SISMEMBER", "aio:boxes:running", box.ContainerName()))
	c.Assert(exists, Equals, true)
	count, _ := redis.Int(conn.Do("SCARD", "aio:boxes:running"))
	c.Assert(count, Equals, 1)

	err = rp.UpdateState(box, "stopped")
	c.Check(err, IsNil)

	// Verify "aio:boxes:running:" set is updated.
	exists, _ = redis.Bool(conn.Do("SISMEMBER", "aio:boxes:running", box.ContainerName()))
	c.Assert(exists, Equals, false)
	count, _ = redis.Int(conn.Do("SCARD", "aio:boxes:running"))
	c.Assert(count, Equals, 0)
}

func (s *ResourcePoolSuite) TestSaveBox(c *C) {
	rp := &ResourcePool{Pool: s.pool}
	conn := s.pool.Get()
	defer conn.Close()
	box1 := makeTestBox(1, "running")
	box2 := makeTestBox(2, "stopped")

	err := rp.SaveBox(box1)
	c.Check(err, IsNil)
	err = rp.SaveBox(box2)
	c.Check(err, IsNil)

	boxJSON, err := redis.Bytes(conn.Do("HGET", "aio:boxes", box1.ContainerName()))
	c.Check(err, IsNil)
	c.Assert(string(boxJSON), Contains, `"id":1`)
	boxJSON, err = redis.Bytes(conn.Do("HGET", "aio:boxes", box2.ContainerName()))
	c.Check(err, IsNil)
	c.Assert(string(boxJSON), Contains, `"id":2`)

	// Verify "aio:boxes:running:" set is updated.
	exists, _ := redis.Bool(conn.Do("SISMEMBER", "aio:boxes:running", box1.ContainerName()))
	c.Assert(exists, Equals, true)
	count, _ := redis.Int(conn.Do("SCARD", "aio:boxes:running"))
	c.Assert(count, Equals, 1)
}

func (s *ResourcePoolSuite) TestFindAllBoxes(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	box1json, _ := utils.ToJSON(makeTestBox(1, "running"))
	box2json, _ := utils.ToJSON(makeTestBox(2, "stopped"))
	conn.Do("HSET", "aio:boxes", "test-1", box1json)
	conn.Do("HSET", "aio:boxes", "test-2", box2json)

	rs := &ResourcePool{Pool: s.pool}
	boxes, _ := rs.FindAllBoxes()

	c.Assert(len(boxes), Equals, 2)
	c.Assert(boxes[0].Id, Equals, 2)
	c.Assert(boxes[0].State, Equals, "stopped")
	c.Assert(boxes[1].Id, Equals, 1)
	c.Assert(boxes[1].State, Equals, "running")
}

func (s *ResourcePoolSuite) TestFindBoxesByState(c *C) {
	conn := s.pool.Get()
	defer conn.Close()
	box1json, _ := utils.ToJSON(makeTestBox(1, "running"))
	box2json, _ := utils.ToJSON(makeTestBox(2, "stopped"))
	box3json, _ := utils.ToJSON(makeTestBox(3, "running"))
	conn.Do("HSET", "aio:boxes", "test-1", box1json)
	conn.Do("HSET", "aio:boxes", "test-2", box2json)
	conn.Do("HSET", "aio:boxes", "test-3", box3json)

	rs := &ResourcePool{Pool: s.pool}
	boxes, _ := rs.FindBoxesByState("running")

	c.Assert(len(boxes), Equals, 2)
	c.Assert(boxes[0].Id, Equals, 3)
	c.Assert(boxes[0].State, Equals, "running")
	c.Assert(boxes[1].Id, Equals, 1)
	c.Assert(boxes[1].State, Equals, "running")

	boxes, _ = rs.FindBoxesByState("stopped")

	c.Assert(len(boxes), Equals, 1)
	c.Assert(boxes[0].Id, Equals, 2)
	c.Assert(boxes[0].State, Equals, "stopped")
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
	boxes, _ := rs.FindRunningBoxes()

	c.Assert(len(boxes), Equals, 2)
	c.Assert(boxes[0].Id, Equals, 3)
	c.Assert(boxes[0].State, Equals, "running")
	c.Assert(boxes[1].Id, Equals, 1)
	c.Assert(boxes[1].State, Equals, "running")
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

func (s *ResourcePoolSuite) TestCountRunningBoxes(c *C) {
	rp := &ResourcePool{Pool: s.pool}
	box1 := makeTestBox(1, "running")
	box2 := makeTestBox(2, "stopped")
	box3 := makeTestBox(3, "running")
	err := rp.SaveBox(box1)
	c.Check(err, IsNil)
	err = rp.SaveBox(box2)
	c.Check(err, IsNil)
	err = rp.SaveBox(box3)
	c.Check(err, IsNil)

	count, _ := rp.CountRunningBoxes()

	c.Assert(count, Equals, 2)
}

func (s *ResourcePoolSuite) TestSetDomain(c *C) {
	conn := s.pool.Get()
	defer conn.Close()

	rs := &ResourcePool{Pool: s.pool}
	rs.SetDomain("wtf.com", "mclovins-container-1")

	containerName, _ := redis.String(conn.Do("HGET", "aio:domains", "wtf.com"))
	c.Assert(containerName, Equals, "mclovins-container-1")
}
