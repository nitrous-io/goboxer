package resource

import (
	"github.com/garyburd/redigo/redis"
)

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
	return resource, nil
}

func (rp *ResourcePool) Release(resource *Resource) (err error) {
	rp.Conn.Send("MULTI")
	rp.Conn.Send("LREM", "aio:used_resources", 0, resource.Ip)
	rp.Conn.Send("RPUSH", "aio:available_resources", resource.Ip)
	_, err = rp.Conn.Do("EXEC")
	return err
}
