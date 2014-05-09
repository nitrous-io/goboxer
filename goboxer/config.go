package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

var VALID_MOUNT = []string{"aufs", "lvm", "overlayfs"}
var VALID_DISK_QUOTA = []string{"prjquota", "usrquota"}

type Config struct {
	Id              string            `json:"id"`
	Vagrant         bool              `json:"vagrant"`
	BoxerChannel    string            `json:"boxerChannel"`
	CoachChannel    string            `json:"coachChannel"`
	RedisMQHost     string            `json:"redisMQHost"`
	RedisMQPort     string            `json:"redisMQPort"`
	RedisStoreHost  string            `json:"redisStoreHost"`
	RedisStorePort  string            `json:"redisStorePort"`
	SelectedDisk    string            `json:"selectedDisk"`
	Disks           []string          `json:"disks"`
	Runtimes        map[string]string `json:"runtimes"`
	MountMethod     string            `json:"mountMethod"`
	QuotaMethod     string            `json:"quotaMethod"`
	CartmanHost     string            `json:"cartmanHost"`
	CartmanPort     string            `json:"cartmanPort"`
	PremiumCPUQuota float64           `json:"premiumCPUQuota"`
	DefaultCPUQuota float64           `json:"defaultCPUQuota"`
	Provisioner     string            `json:"provisioner"`

	S3Region string `json:"s3Region"`
	S3Bucket string `json:"s3Bucket"`

	// TODO These should be in configured via env variables.
	S3AccessKey string `json:"s3AccessKey"`
	S3SecretKey string `json:"s3SecretKey"`
}

func (c Config) RedisMQAddr() string {
	return c.RedisMQHost + ":" + c.RedisMQPort
}

func (c Config) RedisStoreAddr() string {
	return c.RedisStoreHost + ":" + c.RedisStorePort
}

func (c Config) HasValidCartmanSettings() bool {
	return len(c.CartmanHost) > 0 && len(c.CartmanPort) > 0
}

func (c Config) HasValidMountSettings() bool {
	for _, b := range VALID_MOUNT {
		if b == c.MountMethod {
			return true
		}
	}
	return false
}

func (c Config) HasValidDiskQuotaSettings() bool {
	for _, b := range VALID_DISK_QUOTA {
		if b == c.QuotaMethod {
			return true
		}
	}
	return false
}

// reads a json config file and returns back `Config` instance
// configPath - path to json config file
func ParseConfig(configPath string) (config Config, err error) {
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
