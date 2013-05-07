package goboxer

import (
	"github.com/bmizerany/assert"
	"github.com/garyburd/redigo/redis"
	"testing"
)

type ResourcePoolSuite struct {
	conn redis.Conn
}

func newResourcePoolSuite(t *testing.T) *ResourcePoolSuite {
	conn, err := redis.Dial("tcp", ":65000")
	if err != nil {
		t.Fatal("could not connect to redis at port 65000")
	}
	conn.Do("Select", 13)
	conn.Do("FLUSHDB")
	return &ResourcePoolSuite{conn: conn}
}

func (s *ResourcePoolSuite) insertResource(ip, gateway, networkInterface, port string) {
	r := &Resource{Ip: ip, Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	s.conn.Do("RPUSH", "aio:available_resources", ip)
	s.conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *ResourcePoolSuite) tearDown() {
	s.conn.Do("FLUSHDB")
	s.conn.Close()
}

func TestAcquireAvailableResource(t *testing.T) {
	suite := newResourcePoolSuite(t)
	defer suite.tearDown()
	suite.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := &ResourcePool{Conn: suite.conn}
	r, err := rs.Acquire()
	assert.Equal(t, err, nil, "Acquire resource should be success")
	usedResource, err := redis.String(suite.conn.Do("RPOP", "aio:used_resources"))
	assert.Equal(t, usedResource, "192.168.2.2")
	assert.Equal(t, r.Ip, "192.168.2.2")
	assert.Equal(t, r.Gateway, "192.168.2.1")
	assert.Equal(t, r.NetworkInterface, "br-lxc-254")
	assert.Equal(t, r.Port, "10002")
}

func TestAcquireNonAvailableResource(t *testing.T) {
	suite := newResourcePoolSuite(t)
	defer suite.tearDown()
	rs := &ResourcePool{Conn: suite.conn}
	_, err := rs.Acquire()
	assert.NotEqual(t, err, nil)
	suite.tearDown()
}

func TestReleaseValidResource(t *testing.T) {
	suite := newResourcePoolSuite(t)
	defer suite.tearDown()
	suite.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
	rs := &ResourcePool{Conn: suite.conn}
	r, err := rs.Acquire()
	err = rs.Release(r)
	assert.Equal(t, err, nil, "Acquire resource should be success")
	availableResource, err := redis.String(suite.conn.Do("RPOP",
		"aio:available_resources"))
	assert.Equal(t, availableResource, "192.168.2.2")
}
