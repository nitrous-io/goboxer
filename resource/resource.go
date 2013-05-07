package resource

import (
	"encoding/json"
)

type Resource struct {
	Ip               string `json:"ip" redis:"ip"`
	Gateway          string `json:"gateway" redis:"gateway"`
	NetworkInterface string `json:"interface" redis:"interface"`
	Port             string `json:"port" redis:"port"`
}

func (r *Resource) ToJSON() (string, error) {
	b, err := json.Marshal(r)
	return string(b), err
}
