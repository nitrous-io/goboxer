// Handle messages that goes in/out of boxer via redis channel
package runtime

import (
	"github.com/bitly/go-simplejson"
	"github.com/garyburd/redigo/redis"
)

type Handler func(m *simplejson.Json) (Message, error)

type Runtime struct {
	BoxerConn redis.Conn
	CoachConn redis.Conn
	BoxerPSC  redis.PubSubConn
	Handles   map[string]Handler
}

// TODO: refactor aio-boxer-channel and aio-coach-channel out
func (r *Runtime) Start() {
	r.BoxerPSC = redis.PubSubConn{r.BoxerConn}

	r.BoxerPSC.Subscribe("aio-boxer-channel")
	go func() {
		for {
			switch n := r.BoxerPSC.Receive().(type) {
			// message is a json-encoded string of `Message` type
			// we are going to match it with the `Handles` which knows how to
			// process each message
			case redis.Message:
				json, err := simplejson.NewJson(n.Data)
				messageType, err := json.Get("type").String()
				if err != nil {
					r.CoachConn.Do("PUBLISH", "aio-coach-channel", "failed")
				}
				handle, ok := r.Handles[messageType]
				if ok {
					message, err := handle(json)
					if err != nil {
						r.CoachConn.Do("PUBLISH", "aio-coach-channel", "failed")
					} else {
						messageJSON, err := MessageJSON(message)
						if err != nil {
							r.CoachConn.Do("PUBLISH", "aio-coach-channel", "failed")
						} else {
							r.CoachConn.Do("PUBLISH", "aio-coach-channel", messageJSON)
						}
					}
				} else {
					r.CoachConn.Do("PUBLISH", "aio-coach-channel", "failed")
				}
			case redis.Subscription:
				if n.Count == 0 {
					return
				}
			}
		}
	}()
}

func (r *Runtime) End() {
	r.BoxerPSC.Unsubscribe()
}
