// benchmark goboxer's create operations
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nitrous-io/goboxer"
	"github.com/garyburd/redigo/redis"
	"strconv"
	"sync"
	"time"
)

const (
	BOXER_CHANNEL = "aio-boxer-channel"
	COACH_CHANNEL = "aio-coach-channel"
)

type benchmarkResult map[string]float64

var config *benchmarkConfig

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

func main() {
	// initialize number of boxes
	flNumBox := flag.Int("n", 0, "number of boxes, override config")
	flConfigPath := flag.String("c", "./config.json", "config path")
	flag.Parse()

	config = NewConfig(*flConfigPath)

	if *flNumBox > 0 {
		config.NumBox = *flNumBox
	}

	// initialize number of boxes
	idsMap := []int{}
	for i := 0; i < config.NumBox; i++ {
		id := i + 2
		idsMap = append(idsMap, id)
	}

	redisPool := goboxer.NewRedisPool(":6379")
	pubConn := redisPool.Get()
	defer pubConn.Close()
	insertResource(idsMap, pubConn)

	subConn := redisPool.Get()
	defer subConn.Close()

	psc := redis.PubSubConn{Conn: subConn}
	psc.Subscribe(COACH_CHANNEL)

	start := time.Now()
	var wg sync.WaitGroup
	// subscribe to coach
	go func() {
		pubConn := redisPool.Get()
		defer pubConn.Close()
		for {
			switch v := psc.Receive().(type) {
			case redis.Message:
				message := goboxer.Message{}
				if err := json.Unmarshal(v.Data, &message); err != nil {
					fmt.Println("Failed to decode redis message", string(v.Data))
				}
				if message.Type == "aio:created" {
					m := startMessage(message.BoxId)
					_, err := pubConn.Do("PUBLISH", BOXER_CHANNEL, m)
					if err != nil {
						fmt.Println("Err", err)
					}
				} else if message.Type == "aio:started" {
					dm := destroyMessage(message.BoxId)
					_, err := pubConn.Do("PUBLISH", BOXER_CHANNEL, dm)
					if err != nil {
						fmt.Println("Err", err)
						panic(`cleanup with
						umount /var/lib/lxc/mclovins-container-{1..20}
						umount /lxc/disk2/mclovins-container-{1..20}
						rm -rf /var/lib/lxc/mclovins-container*
						rm -rf /lxc/disk2/mclovins-container*`)
					}
				} else if message.Type == "aio:destroyed" {
					// mark the sequence for this box as finished
					wg.Done()
				}
			}
		}
	}()

	// publish creation message via redis
	for _, id := range idsMap {
		wg.Add(1)
		go func(id int) {
			pubConn := redisPool.Get()
			defer pubConn.Close()
			m := createMessage(id)
			pubConn.Do("PUBLISH", BOXER_CHANNEL, m)
		}(id)
	}
	wg.Wait()
	fmt.Println("------> Complete: ", time.Since(start).Seconds())
}

// return json-formatted message to create a box
func createMessage(id int) string {
	m := goboxer.Message{
		Type:       "aio:create",
		Id:         config.BoxerId,
		BoxId:      id,
		Name:       config.BoxName,
		Runtime:    config.Runtime,
		Memory:     "384M",
		DiskSpace:  "750M",
		PublicKeys: []string{"ssh-rsa qwer", "ssh-rsa asdf"},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// return json-formatted message to start a box
func startMessage(id int) string {
	m := goboxer.Message{
		Type:  "aio:start",
		Id:    config.BoxerId,
		BoxId: id,
		Name:  config.BoxName + "-" + strconv.Itoa(id),
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// return json-formatted message to destroy a box
func destroyMessage(id int) string {
	m := goboxer.Message{
		Type:  "aio:destroy",
		Id:    config.BoxerId,
		BoxId: id,
		Name:  config.BoxName + "-" + strconv.Itoa(id),
	}
	b, _ := json.Marshal(m)
	return string(b)
}
