// Handle messages that goes in/out of boxer
package goboxer

import (
	//"encoding/json"
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
	DataType string      `json:"dataType"`
	Data     interface{} `json:"data"`
	Success  bool        `json:"success"`
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
		Disk:             r.Boxer.SelectedDisk,
		Template:         r.Boxer.Runtimes[m.Runtime],
		Ip:               resource.Ip,
		Gateway:          resource.Gateway,
		NetworkInterface: resource.NetworkInterface,
		Port:             resource.Port,
	}
	if err := r.Boxer.Create(box); !err.Success() {
		r.ResourcePool.Release(resource.Ip)
		msg.Success = false
		return msg, nil
	}
	if err := r.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
		return msg, nil
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
	msg := Message{
		Type:       "aio:started",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	box.State = "running"
	if err := r.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
	}
	return msg, nil
}

func StopBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Stop(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStop: %s", m.Name)
	}
	msg := Message{
		Type:       "aio:stopped",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	box.State = "stopped"
	if err := r.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
	}
	return msg, nil
}

func RestartBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Restart(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToRestart: %s", m.Name)
	}
	msg := Message{
		Type:       "aio:restarted",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
		Success:    true,
	}
	box.State = "running"
	if err := r.ResourcePool.SaveBox(box); err != nil {
		msg.Success = false
	}
	return msg, nil
}

func DestroyBoxHandler(m Message, r *Runtime) (Message, error) {
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	if err := r.Boxer.Destroy(box); !err.Success() {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToDestroy: %s", m.Name)
	}
	if err := r.ResourcePool.Release(box.Ip); err != nil {
		return r.NewInvalidParamMessage(),
			fmt.Errorf("BoxFailedToReleaseResource %s with ip: %s", box.Id, box.Ip)
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
	box, err := r.ResourcePool.FindBox(m.Name)
	if err != nil {
		return r.NewInvalidParamMessage(), fmt.Errorf("BoxNotFound: %s", m.Name)
	}
	message := Message{
		Type:       "aio:configured",
		Id:         r.Id,
		BoxId:      box.Id,
		ReturnVars: m.ReturnVars,
	}
	switch m.DataType {
	case "aio:configure:add_public_key":
		data := (m.Data).(string)
		if err := box.AddPublicKey(data); err != nil {
			return r.NewInvalidParamMessage(), fmt.Errorf("FailedToAddPublicKey: %s", m.Name)
		}
		message.Success = true
		return message, nil
	case "aio:configure:remove_public_key":
		data := (m.Data).(string)
		if err := box.RemovePublicKey(data); err != nil {
			return r.NewInvalidParamMessage(), fmt.Errorf("FailedToRemovePublicKey: %s", m.Name)
		}
		message.Success = true
		return message, nil
	case "aio:configure:change_specs":
		data := (m.Data).(map[string]interface{})
		spec := &BoxSpec{}
		spec.Memory = (data["memory"]).(string)
		spec.DiskSpace = (data["diskSpace"]).(string)
		if err := box.ChangeSpec(spec); err != nil {
			return r.NewInvalidParamMessage(), fmt.Errorf("FailedToChangeSpec: %s", m.Name)
		}
		if err := r.Boxer.Restart(box); !err.Success() {
			return r.NewInvalidParamMessage(), fmt.Errorf("BoxFailedToStop: %s", m.Name)
		}
		message.Success = true
		box.State = "running"
		if err := r.ResourcePool.SaveBox(box); err != nil {
			message.Success = false
		}
		return message, nil
	}
	return r.NewInvalidParamMessage(), fmt.Errorf("NotHandled: %s", m.DataType)
}
