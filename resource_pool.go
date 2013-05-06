package goboxer

import (
	"encoding/json"
	"errors"
	"github.com/garyburd/redigo/redis"
)

type ResourcePool struct {
	Conn redis.Conn
}

func (rp *ResourcePool) Acquire() (*Resource, error) {
	var redisResponse interface{}
	var err error
	var resource *Resource
	var ip string
	var resourceJSON []byte
	var ok bool

	rp.Conn.Send("BRPOPLPUSH", "aio:available_resources",
		"aio:used_resources", 1)
	rp.Conn.Flush()
	redisResponse, err = rp.Conn.Receive()
	if err != nil {
		return nil, errors.New("Could Not Acquire Resources")
	}
	if ip, ok = redisResponse.(string); ok {
		rp.Conn.Send("HGETALL", "aio:nwr:"+ip)
		rp.Conn.Flush()
	}
	redisResponse, err = rp.Conn.Receive()
	if err != nil {
		return nil, errors.New("Could Not Acquire Resources")
	}
	if resourceJSON, ok = redisResponse.([]byte); ok {
		err = json.Unmarshal(resourceJSON, resource)
		if err != nil {
			return nil, errors.New("Could Not Acquire Resources")
		}
	}
	return resource, nil
}

func (rp *ResourcePool) Release() (*Resource, error) {
	return nil, nil
}

func NewResourcePool(c redis.Conn) *ResourcePool {
	return &ResourcePool{Conn: c}
}
