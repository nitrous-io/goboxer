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

const VERSION = "0.0.1"
const ADVERTISE_INTERVAL = 5001

var runtimes map[string]string
var handlers map[string]goboxer.MessageHandler

func init() {
	runtimes = map[string]string{
		"rails":  "rails-template",
		"nodejs": "nodejs-template",
		"python": "python-template",
		"go":     "golang-template",
	}
	handlers = map[string]goboxer.MessageHandler{
		"aio:locate": goboxer.LocateBoxHandler,
		"aio:create": goboxer.CreateBoxHandler,
	}
}

type Config struct {
	Id             string `json:"id"`
	BoxerChannel   string `json:"boxerChannel"`
	CoachChannel   string `json:"coachChannel"`
	RedisMQHost    string `json:"redisMQHost"`
	RedisMQPort    string `json:"redisMQPort"`
	RedisStoreHost string `json:"redisStoreHost"`
	RedisStorePort string `json:"redisStorePort"`
}

func (c Config) RedisMQAddr() string {
	return c.RedisMQHost + ":" + c.RedisMQPort
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

	boxer := goboxer.Boxer{Root: "/var/lib/lxc", Runtimes: runtimes}

	boxerRuntime := goboxer.Runtime{
		Id:      config.Id,
		Boxer:   boxer,
		Handles: handlers,
	}

	coachListener := goboxer.CoachListener{
		Runtime:      boxerRuntime,
		BoxerConn:    boxerConn,
		CoachConn:    coachConn,
		BoxerPubSub:  redis.PubSubConn{boxerConn},
		BoxerChannel: config.BoxerChannel,
		CoachChannel: config.CoachChannel,
	}

	fmt.Println("Listening to Coach instruction ...")

	ticker := time.NewTicker(time.Millisecond * ADVERTISE_INTERVAL)
	go func() {
		for t := range ticker.C {
			fmt.Println("Advertise itself to Coach at", t)
			coachListener.Advertise()
		}
	}()

	for {
		coachListener.Listen()
	}
}
