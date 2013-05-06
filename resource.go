package goboxer

import (
	"encoding/json"
)

type Resource struct {
	Ip               string `json:"ip"`
	Gateway          string `json:"gateway"`
	NetworkInterface string `json:"interface"`
	Port             string `json:"port"`
}

func (r *Resource) ToJSON() (string, error) {
	b, err := json.Marshal(r)
	return string(b), err
}
