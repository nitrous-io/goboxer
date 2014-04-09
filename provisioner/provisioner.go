package provisioner

import (
	"fmt"
)

type (
	InitFunc func(root string) (Provisioner, error)

	Provisioner interface {
		Create(name string) (id string, err error)
		Destroy(name, id string) error
		Stop(name, id, uid string) (newId string, err error)
		Start(name, id string, resource *Resource, beforeStart func() error) (newId string, err error)
		GetLxcConfigPath(name, id string) string
		GetRootfsPath(name, id string) string
		IsMounted(name, id string) (bool, error)

		// methods o handle lxc-related operations outside of provisioner
		GetLxcPath(name, id string) string
		GetCurrentRoot(name, id string) string
	}

	OpError struct {
		Op  string // Box operation.
		Err error
	}
)

var (
	NoopCallback       = func(root string) error { return nil }
	DefaultProvisioner string
	provisioners       map[string]InitFunc
)

func init() {
	provisioners = make(map[string]InitFunc)
}

func Register(name string, initFunc InitFunc) error {
	if _, ok := provisioners[name]; ok {
		return fmt.Errorf("Provisioner already registered %s", name)
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

func (e *OpError) Error() string {
	return "provisioner: " + e.Op + ": " + e.Err.Error()
}
