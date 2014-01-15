package main

import (
	"flag"
	"fmt"
	"github.com/action-io/goboxer"
	"github.com/action-io/goboxer/cartman"
	"github.com/garyburd/redigo/redis"
	"os"
	"time"
)

var version string

const ADVERTISE_INTERVAL = 5001

func main() {
	flConfigPath := flag.String("c", "", "Set config path of the program")
	flVersion := flag.Bool("v", false, "Show current version")
	flStartAll := flag.Bool("start-all", false, "Start all containers")
	flag.Parse()
	if *flVersion {
		fmt.Println(version)
		return
	}
	if *flConfigPath == "" {
		fmt.Println("Required a config path. Like ./sample_config.json ?")
		return
	}

	config, err := ParseConfig(*flConfigPath)
	if err != nil {
		panic("Cannot parse config file " + *flConfigPath)
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

	cartman := &cartman.Cartman{
		Host: config.CartmanHost + ":" + config.CartmanPort,
	}
	if !cartman.IsValid() {
		panic("missing cartman configuration")
		os.Exit(1)
	}

	boxer := &goboxer.Boxer{
		Id:              config.Id,
		Vagrant:         config.Vagrant,
		Root:            "/var/lib/lxc",
		SelectedDisk:    config.SelectedDisk,
		Disks:           config.Disks,
		Runtimes:        config.Runtimes,
		ResourcePool:    resourcePool,
		MountMethod:     config.MountMethod,
		QuotaMethod:     config.QuotaMethod,
		Cartman:         cartman,
		PremiumCPUQuota: config.PremiumCPUQuota,
		DefaultCPUQuota: config.DefaultCPUQuota,
		StartTimeout:    time.Duration(config.StartTimeout) * time.Second,
		StopTimeout:     time.Duration(config.StopTimeout) * time.Second,
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
