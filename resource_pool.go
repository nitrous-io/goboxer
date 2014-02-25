package goboxer

import (
	"strconv"

	"github.com/action-io/goboxer/utils"
	"github.com/garyburd/redigo/redis"
)

type Resource struct {
	Ip               string `json:"ip" redis:"ip"`
	Gateway          string `json:"gateway" redis:"gateway"`
	NetworkInterface string `json:"interface" redis:"interface"`
	Port             string `json:"port" redis:"port"`
}

func (r *Resource) GetPort() (port int) {
	port, err := strconv.Atoi(r.Port)
	if err != nil {
		logger.Debugln("Invalid port", r)
	}
	return
}

// Takes care of resource storage such as
// - distribute/release resource
// - save box
// - fetch box
type ResourcePool struct {
	Pool *redis.Pool
}

func (rp *ResourcePool) Acquire() (resource *Resource, err error) {
	var ip string
	var resourceValues []interface{}
	conn := rp.Pool.Get()
	defer conn.Close()

	if ip, err = redis.String(conn.Do("BRPOPLPUSH", "aio:available_resources",
		"aio:used_resources", 1)); err != nil {
		return nil, err
	}
	if resourceValues, err = redis.Values(conn.Do("HGETALL",
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
	conn := rp.Pool.Get()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("LREM", "aio:used_resources", 0, ip)
	conn.Send("RPUSH", "aio:available_resources", ip)
	_, err = conn.Do("EXEC")
	return err
}

// Wrapper around `ResourcePool#SaveBox` so we can update the state in one line
func (rp *ResourcePool) UpdateState(box *Box, state string) error {
	box.State = state
	return rp.SaveBox(box)
}

func (rp *ResourcePool) SaveBox(box *Box) error {
	conn := rp.Pool.Get()
	defer conn.Close()
	json, err := utils.ToJSON(box)
	if err != nil {
		return err
	}
	_, err = conn.Do("HSET", "aio:boxes", box.ContainerName(), json)
	return err
}

func (rp *ResourcePool) FindBox(containerName string) (*Box, error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	return findBox(containerName, conn)
}

func (rp *ResourcePool) DeleteBox(box *Box) (err error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	if err = rp.Release(box.Ip); err != nil {
		return
	}
	_, err = conn.Do("HDEL", "aio:boxes", box.ContainerName())
	return
}

// Find all instance of `Box` that has `containerState` equals to `running` in
// the redis database
func (rp *ResourcePool) FindRunningBoxes() ([]*Box, error) {
	return rp.FindAllBoxes("running")
}

// find all boxes with the specified state. if the state is empty, then return
// all boxes
func (rp *ResourcePool) FindAllBoxes(state string) ([]*Box, error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	var boxes []*Box
	boxNames, err := getBoxNames(conn)
	if err != nil {
		return []*Box{}, err
	}
	for _, name := range boxNames {
		box, err := findBox(name, conn)
		if err != nil {
			return []*Box{}, err
		}
		if len(state) == 0 || box.State == state {
			boxes = append(boxes, box)
		}
	}
	return boxes, nil
}

func (rp *ResourcePool) CountAllBoxes() (int, error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	count, err := redis.Int(conn.Do("HLEN", "aio:boxes"))
	if err != nil {
		return -1, err
	}
	return count, nil
}

func (rp *ResourcePool) CountBoxesByState(state string) (int, error) {
	boxes, err := rp.FindAllBoxes(state)
	if err != nil {
		return -1, err
	}
	return len(boxes), nil
}

func (rp *ResourcePool) CountPremiumBoxes() (int, error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	count := 0
	boxNames, err := getBoxNames(conn)
	if err != nil {
		return -1, err
	}
	for _, name := range boxNames {
		box, err := findBox(name, conn)
		if err != nil {
			return -1, err
		}
		if box.Premium {
			count++
		}
	}
	return count, nil
}

func getBoxNames(conn redis.Conn) ([]string, error) {
	return redis.Strings(conn.Do("HKEYS", "aio:boxes"))
}

func findBox(containerName string, conn redis.Conn) (*Box, error) {
	boxjson, err := redis.Bytes(conn.Do("HGET", "aio:boxes", containerName))
	if err != nil {
		return nil, err
	}
	return BoxFromJSON([]byte(boxjson))
}
