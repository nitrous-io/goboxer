// Take care of resource storage such as
// - distribute/release resource
// - save box
// - fetch box
package goboxer

import (
	"encoding/json"
	"github.com/garyburd/redigo/redis"
)

type Resource struct {
	Ip               string `json:"ip" redis:"ip"`
	Gateway          string `json:"gateway" redis:"gateway"`
	NetworkInterface string `json:"interface" redis:"interface"`
	Port             string `json:"port" redis:"port"`
}

type ResourcePool struct {
	Conn redis.Conn
}

func (rp *ResourcePool) Acquire() (resource *Resource, err error) {
	var ip string
	var resourceValues []interface{}

	if ip, err = redis.String(rp.Conn.Do("BRPOPLPUSH", "aio:available_resources",
		"aio:used_resources", 1)); err != nil {
		return nil, err
	}
	if resourceValues, err = redis.Values(rp.Conn.Do("HGETALL",
		"aio:nwr:"+ip)); err != nil {
		return nil, err
	}
	resource = &Resource{}
	if err = redis.ScanStruct(resourceValues, resource); err != nil {
		return nil, err
	}
	resource.Ip = ip
	return resource, nil
}

func (rp *ResourcePool) Release(resource *Resource) (err error) {
	rp.Conn.Send("MULTI")
	rp.Conn.Send("LREM", "aio:used_resources", 0, resource.Ip)
	rp.Conn.Send("RPUSH", "aio:available_resources", resource.Ip)
	_, err = rp.Conn.Do("EXEC")
	return err
}

func (rp *ResourcePool) SaveBox(box *Box) error {
	json, err := ToJSON(box)
	if err != nil {
		return err
	}
	_, err = rp.Conn.Do("HSET", "aio:boxes", box.ContainerName(), json)
	return err
}

func (rp *ResourcePool) FindBox(containerName string) (*Box, error) {
	box := &Box{}
	boxjson, err := redis.Bytes(rp.Conn.Do("HGET", "aio:boxes", containerName))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(boxjson), box); err != nil {
		return nil, err
	}
	return box, nil
}
