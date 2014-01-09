// test binary to create / destroy a container
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/action-io/goboxer"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"strconv"
)

const (
	BOXER_CHANNEL = "aio-boxer-channel"
	COACH_CHANNEL = "aio-coach-channel"
	BOX_ID        = 2
	BOX_NAME      = "mclovins-test"
	BOXER_ID      = "33.33.33.10"
)

func insertResource(idsMap []int, c redis.Conn) {
	c.Do("FLUSHDB")
	for _, id := range idsMap {
		ip := "192.168.254." + strconv.Itoa(id)
		r := &goboxer.Resource{Gateway: "192.168.254.1",
			NetworkInterface: "br-lxc-254", Port: strconv.Itoa(10000 + id)}
		c.Do("RPUSH", "aio:available_resources", ip)
		c.Do("HMSET", redis.Args{}.Add("aio:nwr:"+ip).AddFlat(r)...)
	}
}

// this requires a running goboxer that we are going to test against
//
// Params of the command are
// -c : id of the box that we are going to create. The full name will be
// mclovins-test-{id}
// -d : delete the box that has the specified id
func main() {
	flCreateOrDestroy := flag.Bool("c", true, "we are going to create a box")
	flag.Parse()

	redisPool := goboxer.NewRedisPool(":6379")
	pubConn := redisPool.Get()
	defer pubConn.Close()

	// initialize 1 resource for us
	id := BOX_ID

	// send the message to goboxer server
	if *flCreateOrDestroy {
		insertResource([]int{id}, pubConn)
		m := createMessage(id)
		pubConn.Do("PUBLISH", BOXER_CHANNEL, m)
		fmt.Println("------> Sent create msg: ", id)
	} else {
		m := destroyMessage(id)
		pubConn.Do("PUBLISH", BOXER_CHANNEL, m)
		fmt.Println("------> Sent destroy msg: ", id)
	}
}

// return json-formatted message to create a box
func createMessage(id int) string {
	// current pubkey
	pubkey, err := ioutil.ReadFile("/home/vagrant/.ssh/id_rsa.pub")
	if err != nil {
		panic("run ssh-keygen -t rsa to get a public key pls")
	}
	m := goboxer.Message{
		Type:       "aio:create",
		Id:         BOXER_ID,
		BoxId:      id,
		Name:       BOX_NAME,
		Runtime:    "test",
		Memory:     "384M",
		DiskSpace:  "750M",
		PublicKeys: []string{string(pubkey)},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// return json-formatted message to destroy a box
func destroyMessage(id int) string {
	m := goboxer.Message{
		Type:  "aio:destroy",
		Id:    BOXER_ID,
		BoxId: id,
		Name:  BOX_NAME + "-" + strconv.Itoa(id),
	}
	b, _ := json.Marshal(m)
	return string(b)
}
