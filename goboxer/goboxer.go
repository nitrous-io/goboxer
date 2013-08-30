package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/action-io/goboxer"
	"github.com/action-io/goboxer/s3"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

var version string

const ADVERTISE_INTERVAL = 5001

type Config struct {
	Id             string            `json:"id"`
	BoxerChannel   string            `json:"boxerChannel"`
	CoachChannel   string            `json:"coachChannel"`
	RedisMQHost    string            `json:"redisMQHost"`
	RedisMQPort    string            `json:"redisMQPort"`
	RedisStoreHost string            `json:"redisStoreHost"`
	RedisStorePort string            `json:"redisStorePort"`
	Runtimes       map[string]string `json:"runtimes"`
	PingHost       string            `json:"pingHost"`
	PingPort       string            `json:"pingPort"`
	AWSAccessKey   string            `json:"awsAccessKey"`
	AWSSecretKey   string            `json:"awsSecretKey"`
	AWSRegion      string            `json:"awsRegion"`
	AWSBucket      string            `json:"awsBucket"`

	// docker configs
	//
	// `DockerSocket`: current active docker daemon socket that will
	// accept box creation
	// `DockerDisks`: map of disk to socket of all docker daemons that are
	// running on the servers. This is useful when we want to redirect
	// non-creation request ( such as restart, stop etc) to existing boxes
	// that may not be under current docker daemon
	//
	// goboxer currently will not try to setup docker instances. This normally
	// should be done as part of a separate process such as upstart
	DockerSocket string            `json:"dockerSocket"`
	DockerDisks  map[string]string `json:"dockerDisks"`
}

func (c Config) RedisMQAddr() string {
	return c.RedisMQHost + ":" + c.RedisMQPort
}

func (c Config) RedisStoreAddr() string {
	return c.RedisStoreHost + ":" + c.RedisStorePort
}

func (c Config) HasValidPingSettings() bool {
	return len(c.PingHost) > 0 && len(c.PingPort) > 0
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
	flBenchmark := flag.Bool("benchmark", false, "Start benchmark mode")
	flBenchmarkStartAll := flag.Bool("benchmark-start-all", false, "Start benchmark mode for start all")
	flNumBox := flag.Int("num", 10, "Number of box that will be created in benchmark mode")
	flag.Parse()
	if *flVersion {
		fmt.Println(version)
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
	if len(config.DockerSocket) == 0 || len(config.DockerDisks) == 0 {
		panic("Require docker socket and disks mapping to be configured")
		os.Exit(1)
	}

	boxerConn, err := redis.Dial("tcp", config.RedisMQAddr())
	if err != nil {
		panic("Cannot connect to redis: " + config.RedisMQAddr())
		os.Exit(1)
	}

	resourcePool := &goboxer.ResourcePool{
		Pool: goboxer.NewRedisPool(config.RedisStoreAddr()),
	}

	boxer := &goboxer.Boxer{
		Id:           config.Id,
		DockerSocket: config.DockerSocket,
		DockerDisks:  config.DockerDisks,
		Runtimes:     config.Runtimes,
		ResourcePool: resourcePool,
		S3Auth:       s3.Auth{config.AWSAccessKey, config.AWSSecretKey},
		S3Region:     config.AWSRegion,
		S3Bucket:     config.AWSBucket,
	}
	boxer.Configure()

	if *flStartAll {
		fmt.Println("Starting all containers ...")
		if err := boxer.Init(); err != nil {
			fmt.Println("Failed to start all boxes", err)
			os.Exit(-1)
			return
		}
		os.Exit(0)
		return
	}

	if *flBenchmarkStartAll {
		if *flNumBox > 0 {
			goboxer.StartBenchmarkAll(boxer, *flNumBox)
		} else {
			fmt.Println("Required -num option in benchmark mode")
			os.Exit(0)
		}
		return
	}

	if *flBenchmark {
		fmt.Println("Benchmarking ...")
		if *flNumBox > 0 {
			goboxer.StartBenchmark(boxer, *flNumBox)
		} else {
			fmt.Println("Required -num option in benchmark mode")
			os.Exit(0)
		}
		return
	}

	// span a go routine that listen and serve ping request
	if config.HasValidPingSettings() {
		pingConfig := goboxer.PingConfig{
			Host:      config.PingHost,
			Port:      config.PingPort,
			MQAddr:    config.RedisMQAddr(),
			StoreAddr: config.RedisStoreAddr(),
		}
		go goboxer.StartPingServer(pingConfig)
	}

	coachListener := &goboxer.CoachListener{
		Boxer:        boxer,
		Pool:         goboxer.NewRedisPool(config.RedisMQAddr()),
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
