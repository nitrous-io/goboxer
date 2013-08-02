// Integration test for goboxer
//
// This does not test how goboxer and coach interact with each other, only test
// the behavior within goboxer itself. Therefore, we dont want to involve
// pubsub testing here
package goboxer

import (
	"encoding/json"
	"fmt"
	"github.com/action-io/goboxer/quota/quotatest"
	"github.com/action-io/goboxer/s3"
	"github.com/garyburd/redigo/redis"
	. "launchpad.net/gocheck"
	"os/exec"
)

type BoxerSuite struct {
	Redis        *redis.Pool
	Boxer        *Boxer
	ResourcePool *ResourcePool
}

var _ = Suite(&BoxerSuite{})

func (s *BoxerSuite) insertResource(ip, gateway, networkInterface, port string) {
	conn := s.Redis.Get()
	defer conn.Close()
	r := &Resource{Gateway: gateway, NetworkInterface: networkInterface, Port: port}
	conn.Do("RPUSH", "aio:available_resources", ip)
	conn.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
}

func (s *BoxerSuite) assertBoxState(c *C, containerName, state string) {
	box, _ := s.ResourcePool.FindBox(containerName)
	c.Assert(box.State, Equals, state)
}

func (s *BoxerSuite) deleteBox(id, name string) Message {
	message := s.Boxer.Receive(Message{
		Type:  "aio:destroy",
		Id:    "testboxer",
		BoxId: id,
		Name:  name + "-" + id,
	})
	return message
}

func (s *BoxerSuite) SetUpTest(c *C) {
	s.Redis = NewRedisPool(":65000")
	conn := s.Redis.Get()
	defer conn.Close()
	conn.Do("FLUSHDB")
	s.insertResource("192.168.254.2", "192.168.254.1", "br-lxc-254", "10002")

	s.ResourcePool = &ResourcePool{Pool: s.Redis}

	s.Boxer = NewBoxer(
		"testboxer",
		"/var/lib/lxc",
		"/lxc/disk1",
		[]string{"/lxc/disk1"},
		map[string]string{
			"test": "nitrous/test",
		},
		s.ResourcePool,
		false,
		s3.AuthTest{},
		"ap-southeast-1",
		"staging-bucket",
	)
}

func (s *BoxerSuite) TearDownTest(c *C) {
	conn := s.Redis.Get()
	defer conn.Close()
	defer s.Redis.Close()
	conn.Do("FLUSHDB")
	exec.Command(DOCKER_BIN, "rmi", "nitrous/1").Run()
}

// TODO: check if the quota is set correctly
func (s *BoxerSuite) TestCreateBox(c *C) {
	message := s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "1M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	c.Assert(message.Type, Equals, "aio:created")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Port, Equals, 10002)
	c.Assert(message.Success, Equals, true)

	root, _ := message.box.DockerContainerPath()
	var checkExists = []string{
		root,
		root + "/rootfs/home/action/workspace",
		root + "/rootfs/home/action/.ssh",
		root + "/rootfs/home/action/.ssh/id_rsa",
		root + "/rootfs/home/action/.ssh/id_rsa.pub",
	}

	var checkContainsContent = map[string][]string{
		root + "/rootfs/etc/passwd":        {"action:x:15001:15001"},
		root + "/rootfs/etc/action/box_id": {"1"},
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-key QWER",
			"ssh-key ASDF",
		},
		root + "/config.lxc": {
			"lxc.utsname = mclovins-container-1",
			fmt.Sprintf("lxc.cgroup.memory.limit_in_bytes = %d", 1024*1024),
			fmt.Sprintf("lxc.cgroup.memory.memsw.limit_in_bytes = %d", 2*1024*1024),
		},
	}

	var checkPermissions = map[string][]int{
		root + "/rootfs/home/action":                 {15001, 15001},
		root + "/rootfs/home/action/workspace":       {15001, 15001},
		root + "/rootfs/home/action/.ssh":            {15001, 15001},
		root + "/rootfs/home/action/.ssh/id_rsa":     {15001, 15001},
		root + "/rootfs/home/action/.ssh/id_rsa.pub": {15001, 15001},
	}

	DoesExist(c, checkExists)
	DoesContain(c, checkContainsContent)
	DoesHavePermissions(c, checkPermissions)

	// check redis
	conn := s.Redis.Get()
	defer conn.Close()
	box, _ := s.ResourcePool.FindBox("mclovins-container-1")
	c.Assert(box.DockerId, Equals, message.box.DockerId)

	// Quota
	limit, _ := s.Boxer.Quota.ViewLimit("1", "/var/lib/docker")
	c.Assert(limit.Limit, Equals, 1024*1024)
}

func (s *BoxerSuite) TestStop(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	message := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:stop",
		Name:  "mclovins-container-1",
	})
	defer s.deleteBox("1", "mclovins-container")

	c.Assert(message.Type, Equals, "aio:stopped")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	info, _ := message.box.dockerInfo()
	c.Assert(info.State.Running, Equals, false)
}

func (s *BoxerSuite) TestMigrate(c *C) {
	message := s.Boxer.Receive(Message{
		Id:          "testboxer",
		BoxId:       "1",
		Type:        "aio:migrate",
		DataType:    "aio:migrate:create",
		Name:        "mclovins-container",
		Runtime:     "test",
		SnapshotURL: "s3://ap-southeast-1/staging-apse1.action.io/testboxer-1234.tar.gz",
		Memory:      "1M",
		DiskSpace:   "1024M",
		PublicKeys:  []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	c.Assert(message.Success, Equals, true)
	c.Assert(message.Type, Equals, "aio:migrate")
	c.Assert(message.DataType, Equals, "aio:migrate:created")

	// it contains the content of the snapshot
	root, _ := message.box.DockerContainerPath()
	DoesExist(c, []string{
		root + "/rootfs/home/action/test.txt",
	})

	var checkContainsContent = map[string][]string{
		root + "/rootfs/etc/passwd":        {"action:x:15001:15001"},
		root + "/rootfs/etc/action/box_id": {"1"},
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-rsa QWER",
			"ssh-rsa ASDF",
		},
		root + "/config.lxc": {
			"lxc.utsname = mclovins-container-1",
			fmt.Sprintf("lxc.cgroup.memory.limit_in_bytes = %d", 1024*1024),
			fmt.Sprintf("lxc.cgroup.memory.memsw.limit_in_bytes = %d", 2*1024*1024),
		},
	}

	var checkPermissions = map[string][]int{
		root + "/rootfs/home/action": {15001, 15001},
	}

	DoesContain(c, checkContainsContent)
	DoesHavePermissions(c, checkPermissions)

	// check redis
	conn := s.Redis.Get()
	defer conn.Close()
	box, _ := s.ResourcePool.FindBox("mclovins-container-1")
	c.Assert(box.DockerId, Equals, message.box.DockerId)

	// Quota
	limit, _ := s.Boxer.Quota.ViewLimit("1", "/var/lib/docker")
	c.Assert(limit.Limit, Equals, 1024*1024)
}

func (s *BoxerSuite) TestStart(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")
	message := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:start",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:started")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	info, _ := message.box.dockerInfo()
	c.Assert(info.State.Running, Equals, true)
}

func (s *BoxerSuite) TestDestroy(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	message := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:destroy",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:destroyed")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	_, err := message.box.dockerInfo()
	c.Assert(err, NotNil)

	// check redis
	conn := s.Redis.Get()
	defer conn.Close()
	// box should be deleted from the collection
	res, _ := conn.Do("HGET", "aio:boxes", "mclovins-container-1")
	c.Assert(res, IsNil)
}

func (s *BoxerSuite) TestRestart(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")
	message := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:restart",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:restarted")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	info, _ := message.box.dockerInfo()
	c.Assert(info.State.Running, Equals, true)
}

func (s *BoxerSuite) TestAddPublicKey(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:add_public_key",
		Data:     "ssh-rsa publickey",
		Name:     "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.DataType, Equals, "aio:configured:add_public_key")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root, _ := message.box.DockerContainerPath()
	var checkContainsContent = map[string][]string{
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-rsa QWER",
			"ssh-rsa ASDF",
			"ssh-rsa publickey",
		},
	}
	DoesContain(c, checkContainsContent)
}

func (s *BoxerSuite) TestRemovePublicKey(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:remove_public_key",
		Data:     "ssh-rsa QWER",
		Name:     "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.DataType, Equals, "aio:configured:remove_public_key")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root, _ := message.box.DockerContainerPath()
	var checkContent = map[string][]string{
		root + "/rootfs/etc/ssh/univ_public_keys": {
			"ssh-rsa QWER",
		},
	}
	DoesNotContain(c, checkContent)
}

func (s *BoxerSuite) TestChangeBoxSpecs(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "1M",
		DiskSpace:  "1024M",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	limit, _ := s.Boxer.Quota.ViewLimit("1", "/var/lib/docker")
	c.Assert(limit.Limit, Equals, 1024*1024)

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:change_specs",
		Data: map[string]interface{}{
			"memory":    "2M",
			"diskSpace": "2048M",
		},
		Name: "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)
	c.Assert(message.DataType, Equals, "aio:configured:change_specs")
	data := (message.Data).(map[string]interface{})
	currentMemory, _ := (data["memory"]).(string)
	currentDiskSpace, _ := (data["diskSpace"]).(string)
	// memory is set successfully
	c.Assert(currentMemory, Equals, "2M")
	// disk space cant be set so we still use the old value
	c.Assert(currentDiskSpace, Equals, "2048M")

	root, _ := message.box.DockerContainerPath()
	var checkContent = map[string][]string{
		root + "/config.lxc": {
			fmt.Sprintf("lxc.cgroup.memory.limit_in_bytes = %d", 2*1024*1024),
			fmt.Sprintf("lxc.cgroup.memory.memsw.limit_in_bytes = %d", 4*1024*1024),
		},
	}
	DoesContain(c, checkContent)

	limit, _ = s.Boxer.Quota.ViewLimit("1", "/var/lib/docker")
	c.Assert(limit.Limit, Equals, 2*1024*1024)

	info, _ := message.box.dockerInfo()
	c.Assert(info.State.Running, Equals, true)
}

func (s *BoxerSuite) TestNewAdvertiseMessage(c *C) {
	m := s.Boxer.NewAdvertiseMessage()
	c.Assert(m.Runtimes, DeepEquals, []string{"test"})
	s.Boxer.Runtimes = map[string]string{
		"test": "test_container_template",
		"ruby": "ruby_template",
	}
	m = s.Boxer.NewAdvertiseMessage()
	c.Assert(len(m.Runtimes), Equals, 2)
	c.Assert(SliceInclude(m.Runtimes, "ruby"), Equals, true)
	c.Assert(SliceInclude(m.Runtimes, "test"), Equals, true)
}

func (s *BoxerSuite) TestStartAll(c *C) {
	s.insertResource("192.168.254.3", "192.168.254.1", "br-lxc-254", "10003")
	for _, id := range []string{"1", "2"} {
		s.Boxer.Receive(Message{
			Id:         "testboxer",
			BoxId:      id,
			Type:       "aio:create",
			Name:       "mclovins-container",
			Runtime:    "test",
			Memory:     "256M",
			DiskSpace:  "1G",
			PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
		})
		defer s.deleteBox(id, "mclovins-container")
	}

	AssertLxcState(c, "mclovins-container-1", "STOPPED")
	AssertLxcState(c, "mclovins-container-2", "STOPPED")

	box1, _ := s.ResourcePool.FindBox("mclovins-container-1")
	box2, _ := s.ResourcePool.FindBox("mclovins-container-2")
	s.Boxer.StartAll([]*Box{box1, box2})

	info1, _ := box1.dockerInfo()
	c.Assert(info1.State.Running, Equals, true)
	info2, _ := box1.dockerInfo()
	c.Assert(info2.State.Running, Equals, true)
}

func (s *BoxerSuite) TestInfo(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")
	s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:start",
		Name:  "mclovins-container-1",
	})

	message := s.Boxer.Receive(Message{
		Id:    "testboxer",
		BoxId: "1",
		Type:  "aio:info_request",
		Name:  "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:info_response")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	info, _ := message.box.dockerInfo()
	c.Assert(info.State.Running, Equals, true)
}

func (s *BoxerSuite) TestSetEnvVariables(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		Name:     "mclovins-container-1",
		DataType: "aio:configure:set_env_variables",
		Data: map[string]interface{}{
			"RDS_HOST":     "mysql://insecure",
			"RDS_PASSWORD": "mysql-insecure",
		},
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.DataType, Equals, "aio:configured:set_env_variables")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root, _ := message.box.DockerContainerPath()
	DoesContain(c, map[string][]string{
		root + "/rootfs/etc/environment": []string{
			"RDS_HOST=mysql://insecure",
			"RDS_PASSWORD=mysql-insecure",
		},
	})
}

func (s *BoxerSuite) TestDoubleSetEnvVariables(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		Name:     "mclovins-container-1",
		DataType: "aio:configure:set_env_variables",
		Data: map[string]interface{}{
			"RDS_HOST":     "mysql://insecure",
			"RDS_PASSWORD": "mysql-insecure",
		},
	})

	s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		Name:     "mclovins-container-1",
		DataType: "aio:configure:set_env_variables",
		Data: map[string]interface{}{
			"RDS_HOST":     "mysql://secure",
			"RDS_PASSWORD": "mysql-insecure",
		},
	})

	root := "/lxc/disk1/mclovins-container-1"
	DoesContain(c, map[string][]string{
		root + "/rootfs/etc/environment": []string{
			"RDS_HOST=mysql://secure",
			"RDS_PASSWORD=mysql-insecure",
		},
	})
	DoesNotContain(c, map[string][]string{
		root + "/rootfs/etc/environment": []string{
			"RDS_HOST=mysql://insecure",
		},
	})
}

func (s *BoxerSuite) TestRemoveEnvVariables(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		Name:     "mclovins-container-1",
		DataType: "aio:configure:set_env_variables",
		Data: map[string]interface{}{
			"RDS_HOST":     "mysql://insecure",
			"RDS_PASSWORD": "mysql-insecure",
			"REDIS_HOST":   "127.0.0.1",
		},
	})
	msg := "{\"id\":\"testboxer\",\"name\":\"mclovins-container-1\"," +
		"\"data\":[\"RDS_HOST\", \"RDS_PASSWORD\"]," +
		"\"dataType\":\"aio:configure:remove_env_variables\"" +
		",\"type\":\"aio:configure\"}"
	message := Message{}
	json.Unmarshal([]byte(msg), &message)

	message = s.Boxer.Receive(message)
	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.DataType, Equals, "aio:configured:remove_env_variables")
	c.Assert(message.Id, Equals, "testboxer")
	c.Assert(message.BoxId, Equals, "1")
	c.Assert(message.Success, Equals, true)

	root, _ := message.box.DockerContainerPath()
	DoesContain(c, map[string][]string{
		root + "/rootfs/etc/environment": []string{
			"REDIS_HOST=127.0.0.1",
		},
	})
	DoesNotContain(c, map[string][]string{
		root + "/rootfs/etc/environment": []string{
			"RDS_HOST=mysql://insecure",
			"RDS_PASSWORD=mysql-insecure",
		},
	})
}

func (s *BoxerSuite) TestInvalidConfigureDataDoesNotPanic(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		Name:     "mclovins-container-1",
		DataType: "aio:configure:set_env_variables",
		Data: []string{
			"so wrong",
			"i dont know what to say",
		},
	})

	c.Assert(message.Type, Equals, "aio:invalid:params")
}

func (s *BoxerSuite) TestChangeBoxSpecsFail(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "1M",
		DiskSpace:  "1024M",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	limit, _ := s.Boxer.Quota.ViewLimit("1", "/var/lib/docker")
	c.Assert(limit.Limit, Equals, 1024*1024)

	// swap Quota with FailQuota so all quota calls will return errors
	oldQuota := s.Boxer.Quota
	s.Boxer.Quota = quotatest.FailQuota{}

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:change_specs",
		Data: map[string]interface{}{
			"memory":    "2M",
			"diskSpace": "2048M",
		},
		Name: "mclovins-container-1",
	})

	c.Assert(message.Type, Equals, "aio:configured")
	c.Assert(message.Success, Equals, false)
	c.Assert(message.DataType, Equals, "aio:configured:change_specs")
	data := (message.Data).(map[string]interface{})
	currentMemory, _ := (data["memory"]).(string)
	currentDiskSpace, _ := (data["diskSpace"]).(string)
	// memory is set successfully
	c.Assert(currentMemory, Equals, "2M")
	// disk space cant be set so we still use the old value
	c.Assert(currentDiskSpace, Equals, "1024M")

	// Swap back the original Quota so the next test that uses Quota will be
	// normal
	s.Boxer.Quota = oldQuota

	// it still sets the new memory
	root, _ := message.box.DockerContainerPath()
	var checkContent = map[string][]string{
		root + "/config.lxc": {
			fmt.Sprintf("lxc.cgroup.memory.limit_in_bytes = %d", 2*1024*1024),
			fmt.Sprintf("lxc.cgroup.memory.memsw.limit_in_bytes = %d", 4*1024*1024),
		},
	}
	DoesContain(c, checkContent)

	// it cant set the new quota
	newLimit, _ := s.Boxer.Quota.ViewLimit("1", "/var/lib/docker")
	c.Assert(newLimit.Limit, Equals, 1024*1024)
}

func (s *BoxerSuite) TestChangeBoxSpecsInvalidParams(c *C) {
	s.Boxer.Receive(Message{
		Id:         "testboxer",
		BoxId:      "1",
		Type:       "aio:create",
		Name:       "mclovins-container",
		Runtime:    "test",
		Memory:     "256M",
		DiskSpace:  "1024M",
		PublicKeys: []string{"ssh-rsa QWER", "ssh-rsa ASDF"},
	})
	defer s.deleteBox("1", "mclovins-container")

	message := s.Boxer.Receive(Message{
		Id:       "testboxer",
		BoxId:    "1",
		Type:     "aio:configure",
		DataType: "aio:configure:change_specs",
		Data: map[string]interface{}{
			"memory":    512,
			"diskSpace": 2048,
		},
		Name: "mclovins-container-1",
	})

	c.Assert(message.Success, Equals, false)
	data := (message.Data).(map[string]interface{})
	currentMemory, _ := (data["memory"]).(string)
	currentDiskSpace, _ := (data["diskSpace"]).(string)
	c.Assert(currentMemory, Equals, "256M")
	c.Assert(currentDiskSpace, Equals, "1024M")
}
