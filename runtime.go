// Handle messages that goes in/out of boxer
package goboxer

import (
	"encoding/json"
	"fmt"
	"github.com/action-io/goboxer/host"
	"github.com/bitly/go-simplejson"
	"github.com/garyburd/redigo/redis"
)

// we concat all messages type here and use `Receive` to distribute correct
// type to correct function for now
type Message struct {
	// all messages can have these
	Id         string            `json:"id"`
	BoxId      string            `json:"boxId"`
	Type       string            `json:"type"`
	ReturnVars map[string]string `json:"returnVars"`

	// for advertise message
	FreeMemory int               `json:"freeMemory"`
	Runtimes   map[string]string `json:"runtimes"`

	// for create message
	Name        string   `json:"name"`
	Runtime     string   `json:"runtime"`
	SnapshotURL string   `json:"snapshotURL"`
	Memory      int      `json:"memory"`
	DiskSpace   string   `json:"diskSpace"`
	PublicKeys  []string `json:"publicKeys"`
	SshKeyPair  []string `json:"sshKeyPair"`

	// for normal message
	DataType string `json:"dataType"`
	Data     string `json:"data"`
	Success  bool   `json:"success"`
}

func MessageJSON(m Message) (string, error) {
	b, err := json.Marshal(m)
	return string(b), err
}

// a handler will receive message and runtime, do some work with it and return
// a new message
type MessageHandler func(m *simplejson.Json, r *Runtime) (Message, error)

type Runtime struct {
	Id           string
	Boxer        *Boxer
	ResourcePool *ResourcePool
	Handles      map[string]MessageHandler
}

func (r *Runtime) Receive(json *simplejson.Json) (Message, error) {
	messageType, err := json.Get("type").String()
	if err != nil {
		return Message{}, fmt.Errorf("Invalid Message: missing type")
	}
	handle, ok := r.Handles[messageType]
	if !ok {
		return Message{}, fmt.Errorf("Invalid Message: not able to handle %s", messageType)
	}
	return handle(json, r)
}

func (r *Runtime) NewAdvertiseMessage() Message {
	return Message{
		Type:       "aio:advertise",
		Id:         r.Id,
		Runtimes:   r.Boxer.Runtimes,
		FreeMemory: int(host.FreeMemory()),
	}
}

func (r *Runtime) NewInvalidParamMessage() Message {
	return Message{
		Type:    "aio:invalid:params",
		Id:      r.Id,
		Success: false,
	}
}

func LocateBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	messageType, err := m.Get("type").String()
	if err != nil {
		return Message{}, err
	}
	if messageType != "aio:locate" {
		return Message{}, fmt.Errorf("Expected aio:locate but got: %s", messageType)
	}
	return r.NewAdvertiseMessage(), nil
}

func CreateBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	messageType, err := m.Get("type").String()
	if err != nil {
		return Message{}, err
	}
	if messageType != "aio:create" {
		return Message{}, fmt.Errorf("Expected aio:locate but got: %s", messageType)
	}
	config = &Config{
		Id:          m.Get("boxId").Int(),
		Name:        m.Get("name").String(),
		Runtime:     m.Get("runtime").String(),
		Memory:      m.Get("memory").String(),
		DiskSpace:   m.Get("diskSpace").String(),
		PublicKeys:  m.Get("publicKeys").Array(),
		SnapshotURL: m.Get("snapshotUrl").String(),
	}

	box, err := r.Boxer.Create()
	return Message{}, nil
}

func StartBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	return Message{}, nil
}

func StopBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	return Message{}, nil
}

func RestartBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	return Message{}, nil
}

func DestroyBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	return Message{}, nil
}

func ConfigureBoxHandler(m *simplejson.Json, r *Runtime) (Message, error) {
	return Message{}, nil
}
