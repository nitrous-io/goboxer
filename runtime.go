// Handle messages that goes in/out of boxer
package goboxer

import (
	"fmt"
	"github.com/action-io/goboxer/host"
)

var runtimes []string

func init() {
	runtimes = []string{
		"rails",
		"nodejs",
		"django",
		"golang",
		"test",
	}
}

// we concat all messages type here and use `Receive` to distribute correct
// type to correct function for now
type Message struct {
	// all messages can have these
	Id         string            `json:"id"`
	BoxId      string            `json:"boxId"`
	Type       string            `json:"type"`
	ReturnVars map[string]string `json:"returnVars"`

	// for advertise message
	FreeMemory int      `json:"freeMemory"`
	Runtimes   []string `json:"runtimes"`

	// for create message
	// Name is used subsequently in coach message to describe containerName
	// i.e. 123-testbox
	Name        string   `json:"name"`
	Port        string   `json:"port"`
	Runtime     string   `json:"runtime"`
	SnapshotURL string   `json:"snapshotURL"`
	Memory      string   `json:"memory"`
	DiskSpace   string   `json:"diskSpace"`
	PublicKeys  []string `json:"publicKeys"`
	SshKeyPair  []string `json:"sshKeyPair"`

	// for normal message
	DataType string `json:"dataType"`
	Data     string `json:"data"`
	Success  bool   `json:"success"`
}

// a handler will receive message and runtime, do some work with it and return
// a new message
type MessageHandler func(m Message, r *Runtime) (Message, error)

type Runtime struct {
	Id           string
	Boxer        *Boxer
	ResourcePool *ResourcePool
	Handles      map[string]MessageHandler
}

func (r *Runtime) Receive(msg Message) (Message, error) {
	handle, ok := r.Handles[msg.Type]
	if !ok {
		return Message{}, fmt.Errorf("Invalid Message: unknown type %s", msg.Type)
	}
	msg.Id = r.Id
	return handle(msg, r)
}

func (r *Runtime) NewAdvertiseMessage() Message {
	return Message{
		Type:       "aio:advertise",
		Id:         r.Id,
		Runtimes:   runtimes,
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

func LocateBoxHandler(m Message, r *Runtime) (Message, error) {
	return r.NewAdvertiseMessage(), nil
}

func CreateBoxHandler(m Message, r *Runtime) (Message, error) {
	msg := Message{
		Type:  "aio:created",
		Id:    r.Id,
		BoxId: m.BoxId,
	}
	resource, err := r.ResourcePool.Acquire()
	if err != nil {
		msg.Success = false
		return msg, nil
	}
	box := &Box{
		Id:               m.BoxId,
		BoxerId:          m.Id,
		Name:             m.Name,
		Runtime:          m.Runtime,
		Memory:           m.Memory,
		DiskSpace:        m.DiskSpace,
		PublicKeys:       m.PublicKeys,
		SnapshotURL:      m.SnapshotURL,
		Template:         r.Boxer.Runtimes[m.Runtime],
		Ip:               resource.Ip,
		Gateway:          resource.Gateway,
		NetworkInterface: resource.NetworkInterface,
		Port:             resource.Port,
	}
	if err := r.Boxer.Create(box); !err.Success() {
		r.ResourcePool.Release(resource)
		msg.Success = false
		return msg, nil
	}
	if err := r.ResourcePool.SaveBox(box); err != nil {
		Debug("FailedToSaveBox", err)
	}
	msg.Port = box.Port
	msg.Success = true
	return msg, nil
}

func StartBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Start(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStart: %s", m.Name)
	}
	return Message{
		Type:       "aio:started",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}, nil
}

func StopBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Stop(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStop: %s", m.Name)
	}
	return Message{
		Type:       "aio:stopped",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}, nil
}

func RestartBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Restart(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToRestart: %s", m.Name)
	}
	return Message{
		Type:       "aio:restarted",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}, nil
}

func DestroyBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Destroy(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToDestroy: %s", m.Name)
	}
	return Message{
		Type:       "aio:destroyed",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}, nil
}

func ConfigureBoxHandler(m Message, r *Runtime) (Message, error) {
	return Message{}, nil
}
