// Handle messages that goes in/out of boxer
package goboxer

import (
	"fmt"
	"github.com/bitly/go-simplejson"
)

type MessageHandler func(m *simplejson.Json) (Message, error)

type Runtime struct {
	Handles map[string]MessageHandler
}

func (r *Runtime) Receive(json *simplejson.Json) (Message, error) {
	messageType, err := json.Get("type").String()
	if err != nil {
		return nil, fmt.Errorf("Invalid Message: missing type")
	}
	handle, ok := r.Handles[messageType]
	if !ok {
		return nil, fmt.Errorf("Invalid Message: not able to handle %s", messageType)
	}
	return handle(json)
}
