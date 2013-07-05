// HTTP Ping server for goboxer
package goboxer

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"net/http"
	"sync"
)

// interface that supports redis.Conn so we can stub it out in test
type testConn interface {
	Do(commandName string, args ...interface{}) (interface{}, error)
	Close() error
}

func pingHandler(mqConn, storeConn testConn) func(http.ResponseWriter, *http.Request) {
	mutex := sync.Mutex{}
	return func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		_, mqErr := mqConn.Do("PING")
		_, storeErr := storeConn.Do("PING")
		logger.Debugln(mqErr, storeErr)
		if mqErr != nil || storeErr != nil {
			msg := fmt.Sprintf("mq: %v, store: %v", mqErr, storeErr)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "ok")
	}
}

// This creates an embeded http server at <host>:<port> and expose "/"
// endpoint, which return success when the redis connections are responsive
func startPingServer(host, port string, mqConn, storeConn redis.Conn) error {
	http.HandleFunc("/", pingHandler(mqConn, storeConn))
	return http.ListenAndServe(host+":"+port, nil)
}
