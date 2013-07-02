// Only contains the benchmark test for boxer
package goboxer

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
	"strconv"
	"sync"
)

const NUM_BOX = 10

type BenchmarkSuite struct {
	Redis        redis.Conn
	Boxer        *Boxer
	ResourcePool *ResourcePool
	Resources    []*Resource
}

var _ = Suite(&BenchmarkSuite{})

func (s *BenchmarkSuite) insertResource(ip, gateway, networkInterface, port string) {
	r := &Resource{Ip: ip, Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	s.Resources = append(s.Resources, r)
	s.Redis.Do("RPUSH", "aio:available_resources", r.Ip)
	s.Redis.Do("HMSET", redis.Args{}.Add("aio:nwr:"+r.Ip).AddFlat(r)...)
}

func (s *BenchmarkSuite) assertBoxState(c *C, containerName, state string) {
	box, _ := s.ResourcePool.FindBox(containerName)
	c.Assert(box.State, Equals, state)
}

func (s *BenchmarkSuite) deleteBox(id, name string) Message {
	message := s.Boxer.Receive(Message{
		Type:  "aio:destroy",
		Id:    "testboxer",
		BoxId: id,
		Name:  name + "-" + id,
	})
	return message
}

func (s *BenchmarkSuite) SetUpTest(c *C) {
	s.Redis, _ = redis.Dial("tcp", ":65000")
	s.Redis.Do("Select", 13)
	s.Redis.Do("FLUSHDB")

	for i := 2; i <= NUM_BOX; i++ {
		is := strconv.Itoa(i)
		s.insertResource("192.168.253."+is, "192.168.243.1", "br-lxc-253",
			strconv.Itoa(10000+i))
	}

	s.ResourcePool = &ResourcePool{Conn: s.Redis}
	s.Boxer = NewBoxer(
		"testboxer",
		"/var/lib/lxc",
		"/lxc/disk1",
		[]string{"/lxc/disk1"},
		map[string]string{
			"test": "test_container_template",
		},
		s.ResourcePool,
	)
}

func (s *BenchmarkSuite) TearDownTest(c *C) {
	s.Redis.Do("FLUSHDB")
	s.Redis.Close()
}

func (s *BenchmarkSuite) BenchmarkCreate(c *C) {
	var wg sync.WaitGroup
	for i, _ := range s.Resources {
		i = i + 1
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			istr := strconv.Itoa(i)
			message := s.Boxer.Receive(Message{
				Id:         "testboxer",
				BoxId:      istr,
				Type:       "aio:create",
				Name:       "mclovins-container",
				Runtime:    "test",
				Memory:     "256M",
				DiskSpace:  "1G",
				PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
			})

			c.Assert(message.Type, Equals, "aio:created")
			c.Assert(message.BoxId, Equals, istr)
			c.Assert(message.Success, Equals, true)

			root := "/lxc/disk1/mclovins-container-" + istr
			var checkExists = []string{
				"/var/lib/lxc/mclovins-container-" + istr,
				root,
				root + "_delta",
			}

			var checkContainsContent = map[string][]string{
				"/etc/projid":   {fmt.Sprintf("mclovins-container-%s:%s", istr, istr)},
				"/etc/projects": {fmt.Sprintf("%s:/lxc/disk1/mclovins-container-%s", istr, istr)},
			}

			// check the container contents at /lxc/disk1
			DoesExist(c, checkExists)
			DoesContain(c, checkContainsContent)
			Debug("CREATED BOX " + istr)

			s.deleteBox(istr, "mclovins-container")

			Debug("DELETED BOX " + istr)
		}(i)
	}
	wg.Wait()
}
