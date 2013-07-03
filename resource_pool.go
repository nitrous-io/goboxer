// Take care of resource storage such as
// - distribute/release resource
// - save box
// - fetch box
package goboxer

import (
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	"sync"
)

type Resource struct {
	Ip               string `json:"ip" redis:"ip"`
	Gateway          string `json:"gateway" redis:"gateway"`
	NetworkInterface string `json:"interface" redis:"interface"`
	Port             string `json:"port" redis:"port"`
}

type ResourcePool struct {
	Conn redis.Conn
	sync.Mutex
}

func (rp *ResourcePool) Acquire() (resource *Resource, err error) {
	rp.Lock()
	defer rp.Unlock()
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

func (rp *ResourcePool) Release(ip string) (err error) {
	rp.Lock()
	defer rp.Unlock()
	rp.Conn.Send("MULTI")
	rp.Conn.Send("LREM", "aio:used_resources", 0, ip)
	rp.Conn.Send("RPUSH", "aio:available_resources", ip)
	_, err = rp.Conn.Do("EXEC")
	return err
}

// Wrapper around `ResourcePool#SaveBox` so we can update the state in one line
func (rp *ResourcePool) UpdateState(box *Box, state string) error {
	box.State = state
	return rp.SaveBox(box)
}

func (rp *ResourcePool) SaveBox(box *Box) error {
	rp.Lock()
	defer rp.Unlock()
	json, err := ToJSON(box)
	if err != nil {
		return err
	}
	_, err = rp.Conn.Do("HSET", "aio:boxes", box.ContainerName(), json)
	return err
}

func (rp *ResourcePool) FindBox(containerName string) (*Box, error) {
	rp.Lock()
	defer rp.Unlock()
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

// Find all instance of `Box` that has `containerState` equals to `running` in
// the redis database
func (rp *ResourcePool) FindRunningBoxes() ([]*Box, error) {
	var boxes []*Box
	boxNames, err := rp.getBoxNames()
	if err != nil {
		return []*Box{}, err
	}
	for _, name := range boxNames {
		box, err := rp.FindBox(name)
		if err != nil {
			return []*Box{}, err
		}
		if box.State == "running" {
			boxes = append(boxes, box)
		}
	}
	return boxes, nil
}

func (rp *ResourcePool) getBoxNames() ([]string, error) {
	rp.Lock()
	defer rp.Unlock()
	return redis.Strings(rp.Conn.Do("HKEYS", "aio:boxes"))
}
