package goboxer

import (
	"fmt"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

const REDIS_TIMEOUT = 10 * time.Minute
const REDIS_CONN_WAIT = 100 * time.Millisecond

// Create a new redis pool given the redis server address
func NewRedisPool(serverAddr string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return getRedisConn(serverAddr)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

// return the redis connection that we can use
// this wait for the `INFO` call to return something because we want to wait
// until redis data-loading process is done and the server is ready to accept
// connection
//
// If the server take too long ( more than REDIS_TIMEOUT ), then we just give
// up and return an error
func getRedisConn(serverAddr string) (c redis.Conn, err error) {
	if c, err = redis.Dial("tcp", serverAddr); err != nil {
		logger.Println("ERROR: cant connect to redis", err)
		return
	}
	// channel size 1 so if timeout happens, we dont block the channel send
	errChan := make(chan error, 1)
	go func() {
		for {
			info, err := redis.String(c.Do("INFO"))
			if err != nil {
				errChan <- fmt.Errorf("Ready check failed %v", err.Error())
				break
			}
			// check if the info contains loading:0, if yes, then
			// the connection is ready, otherwise, we wait for some time before
			// retrying again
			if strings.Contains(info, "loading:0") {
				errChan <- nil
				break
			} else {
				time.Sleep(REDIS_CONN_WAIT)
				continue
			}
		}
	}()
	select {
	case redisErr := <-errChan:
		err = redisErr
		return
	case <-time.After(REDIS_TIMEOUT):
		// if after 5 minutes we cant get the redis connection, just out
		return nil, err
	}
}
