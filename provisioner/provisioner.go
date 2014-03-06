package provisioner

import (
	"fmt"
)

type InitFunc func(root string) (Provisioner, error)

type CallbackFunc func(data *Data) error

type Data struct {
	Name      string // name of the container
	Id        string // unique id for a container
	RootPath  string // root path of the container
	DeltaPath string // delta path of the container
	Status    string // current status of the container
}

type Provisioner interface {
	Create(name, template string) (*Data, error)
	// beforeStart is called after the container is mounted, but before it is
	// started so we can do more setups to make sure the container data is
	// consistent
	Start(name, id string, beforeStart CallbackFunc) (*Data, error)
	Stop(name, id, uid string) (*Data, error)
	Destroy(name, id string) error
}

type OpError struct {
	Op  string // Box operation.
	Err error
}

func (e *OpError) Error() string {
	return "provisioner: " + e.Op + ": " + e.Err.Error()
}

var (
	NoopCallback       = func(data *Data) error { return nil }
	DefaultProvisioner string
	provisioners       map[string]InitFunc
)

func init() {
	provisioners = make(map[string]InitFunc)
}

func Register(name string, initFunc InitFunc) error {
	if _, ok := provisioners[name]; ok {
		return fmt.Errorf("Name already registered %s", name)
	}
	provisioners[name] = initFunc
	return nil
}

func GetProvisioner(name, root string) (Provisioner, error) {
	if initFunc, exists := provisioners[name]; exists {
		return initFunc(root)
	}
	return nil, fmt.Errorf("ProvisionerNotFound: %s", name)
}
