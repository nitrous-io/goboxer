package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/action-io/goboxer"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

const VERSION = "0.0.5"
const ADVERTISE_INTERVAL = 5001

type Config struct {
	Id             string            `json:"id"`
	BoxerChannel   string            `json:"boxerChannel"`
	CoachChannel   string            `json:"coachChannel"`
	RedisMQHost    string            `json:"redisMQHost"`
	RedisMQPort    string            `json:"redisMQPort"`
	RedisStoreHost string            `json:"redisStoreHost"`
	RedisStorePort string            `json:"redisStorePort"`
	SelectedDisk   string            `json:"selectedDisk"`
	Disks          []string          `json:"disks"`
	Runtimes       map[string]string `json:"runtimes"`
}

func (c Config) RedisMQAddr() string {
	return c.RedisMQHost + ":" + c.RedisMQPort
}

func (c Config) RedisStoreAddr() string {
	return c.RedisStoreHost + ":" + c.RedisStorePort
}

// reads a json config file and returns back `Config` instance
// configPath - path to json config file
func parseConfig(configPath string) (config Config, err error) {
	filePath, err := filepath.Abs(configPath)
	if err != nil {
		return config, err
	}
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(content, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}

func main() {
	flConfigPath := flag.String("c", "", "Set config path of the program")
	flVersion := flag.Bool("v", false, "Show current version")
	flStartAll := flag.Bool("start-all", false, "Start all containers")
	flag.Parse()
	if *flVersion {
		fmt.Println("Current Goboxer Version:", VERSION)
		return
	}
	if *flConfigPath == "" {
		fmt.Println("Required a config path. Like ./sample_config.json ?")
		return
	}

	config, err := parseConfig(*flConfigPath)
	if err != nil {
		panic("Cannot parse config file " + *flConfigPath)
		os.Exit(1)
	}

	boxerConn, err := redis.Dial("tcp", config.RedisMQAddr())
	if err != nil {
		panic("Cannot connect to redis: " + config.RedisMQAddr())
		os.Exit(1)
	}
	coachConn, err := redis.Dial("tcp", config.RedisMQAddr())
	if err != nil {
		panic("Cannot connect to redis: " + config.RedisMQAddr())
		os.Exit(1)
	}
	storeConn, err := redis.Dial("tcp", config.RedisStoreAddr())
	if err != nil {
		panic("Cannot connect to redis: " + config.RedisMQAddr())
		os.Exit(1)
	}

	resourcePool := &goboxer.ResourcePool{Conn: storeConn}

	boxer := goboxer.NewBoxer(
		config.Id,
		"/var/lib/lxc",
		config.SelectedDisk,
		config.Disks,
		config.Runtimes,
		resourcePool,
	)

	if *flStartAll {
		fmt.Println("Starting all containers ...")
		if err := boxer.Init(); err != nil {
			fmt.Println("Failed to start all boxes: %v", err)
			os.Exit(-1)
			return
		}
		os.Exit(0)
		return
	}

	coachListener := &goboxer.CoachListener{
		Boxer:        boxer,
		BoxerConn:    boxerConn,
		CoachConn:    coachConn,
		BoxerPubSub:  redis.PubSubConn{boxerConn},
		BoxerChannel: config.BoxerChannel,
		CoachChannel: config.CoachChannel,
	}

	fmt.Println("Listening to Coach instruction ...")

	ticker := time.NewTicker(time.Millisecond * ADVERTISE_INTERVAL)
	go func() {
		for _ = range ticker.C {
			coachListener.Advertise()
		}
	}()

	coachListener.BoxerPubSub.Subscribe(coachListener.BoxerChannel)
	for {
		coachListener.Listen()
	}
}
