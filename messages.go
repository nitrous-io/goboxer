// contain definition of all messages that handle communication between boxer
// and coach
package goboxer

import (
	"encoding/json"
)

type Message interface{}

type AdvertiseMessage struct {
	id         string            `redis:"id"`
	returnVars map[string]string `redis:"returnVars"`
	freeMemory int               `redis:"freeMemory"`
	runtimes   map[string]string `redis:"runtimes"`
}

type ConfigureMessage struct {
	messageType string            `json:"type"`
	id          string            `json:"id"`
	returnVars  map[string]string `json:"returnVars"`
}

type ConfiguredMessage struct {
}

type CreateMessage struct {
}

type CreatedMessage struct {
}

type DestroyMessage struct {
}

type DestroyedMessage struct {
}

type LocateMessage struct {
}

type RestartMessage struct {
}

type RestartedMessage struct {
}

type StartMessage struct {
}

type StartedMessage struct {
}

type StopMessage struct {
}

type StoppedMessage struct {
}

func MessageJSON(m Message) (string, error) {
	b, err := json.Marshal(m)
	return string(b), err
}
