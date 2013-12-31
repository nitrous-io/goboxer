package goboxer

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"net/http"
)

type PingConfig struct {
	Host      string
	Port      string
	MQAddr    string
	StoreAddr string
}

// interface that supports redis.Conn so we can stub it out in test
type testConn interface {
	Do(commandName string, args ...interface{}) (interface{}, error)
	Close() error
}

func pingHandler(mqConn, storeConn testConn) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		_, mqErr := mqConn.Do("PING")
		_, storeErr := storeConn.Do("PING")
		if mqErr != nil || storeErr != nil {
			msg := fmt.Sprintf("mq: %v, store: %v", mqErr, storeErr)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}
}

// This creates an embeded http server at <host>:<port> and expose "/"
// endpoint, which return success when the redis connections are responsive
func StartPingServer(config PingConfig) error {
	fmt.Println("Start Ping server")
	mqConn, err := redis.Dial("tcp", config.MQAddr)
	if err != nil {
		return fmt.Errorf("Cannot connect to redis: %s", config.MQAddr)
	}
	storeConn, err := redis.Dial("tcp", config.StoreAddr)
	if err != nil {
		return fmt.Errorf("Cannot connect to redis: %s", config.StoreAddr)
	}
	http.HandleFunc("/", pingHandler(mqConn, storeConn))
	return http.ListenAndServe(config.Host+":"+config.Port, nil)
}
