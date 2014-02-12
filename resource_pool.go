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
	if _, err = conn.Do("HSET", "aio:boxes", box.ContainerName(), json); err != nil {
		return err
	}

	// We keep a set of running boxes' names so we can quickly list/count them
	// without going through all boxes.
	op := "SREM"
	if box.State == "running" {
		op = "SADD"
	}
	if _, err = conn.Do(op, "aio:boxes:running", box.ContainerName()); err != nil {
		return err
	}

	return nil
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

func (rp *ResourcePool) SetDomain(domain, container string) (err error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	_, err = conn.Do("HSET", "aio:domains", domain, container)
	return
}

type BoxFinder func(b *Box) bool

// FindBoxes returns instances of Box that satisfy the given BoxFinder func
// (i.e. when given a Box, returns true).
func (rp *ResourcePool) FindBoxes(fn BoxFinder) ([]*Box, error) {
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
		if fn(box) {
			boxes = append(boxes, box)
		}
	}
	return boxes, nil
}

// FindAllBoxes returns all instances of Box.
func (rp *ResourcePool) FindAllBoxes() ([]*Box, error) {
	return rp.FindBoxes(func(b *Box) bool { return true })
}

// FindBoxesByState returns instances of Box in the given state.
func (rp *ResourcePool) FindBoxesByState(state string) ([]*Box, error) {
	return rp.FindBoxes(func(b *Box) bool { return b.State == state })
}

// FindRunningBoxes returns instances of Box in the "running" state.
func (rp *ResourcePool) FindRunningBoxes() ([]*Box, error) {
	return rp.FindBoxesByState("running")
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

func (rp *ResourcePool) CountRunningBoxes() (int, error) {
	conn := rp.Pool.Get()
	defer conn.Close()
	count, err := redis.Int(conn.Do("SCARD", "aio:boxes:running"))
	if err != nil {
		return -1, err
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
