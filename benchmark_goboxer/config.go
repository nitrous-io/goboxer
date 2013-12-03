package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

type benchmarkConfig struct {
	NumBox  int    `json:"numBox"`
	BoxerId string `json:"boxerId"`
	BoxName string `json:"boxName"`
	Runtime string `json:"runtime"`
}

func NewConfig(configPath string) *benchmarkConfig {
	config := &benchmarkConfig{
		NumBox:  10,
		BoxerId: "46.137.252.155",
		BoxName: "mclovins-container",
		Runtime: "test",
	}
	if len(configPath) == 0 {
		return config
	}
	filePath, err := filepath.Abs(configPath)
	if err != nil {
		return config
	}
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return config
	}
	err = json.Unmarshal(content, &config)
	if err != nil {
		return config
	}
	return config
}
