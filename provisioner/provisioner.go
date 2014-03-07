package provisioner

import (
	"fmt"
	"github.com/action-io/goboxer/s3"
)

type InitFunc func(root string) (Provisioner, error)

type CallbackFunc func(root string) error

type Data struct {
	Name      string // name of the container
	Id        string // unique id for a container
	RootPath  string // root path of the container
	DeltaPath string // delta path of the container
	Status    string // current status of the container
}

// Represents settings of a container
type Config struct {
	Id                    string
	Runtime               string
	Premium               bool
	ContainerName         string
	ContainerId           int
	Uid                   string
	Template              string
	Ip                    string
	Gateway               string
	NetworkInterface      string
	MacAddress            string
	Memory                string
	TotalMemory           string
	CpuShares             int
	CpuQuota              float64
	CpuPeriodMicroseconds int
	CpuQuotaMicroseconds  int
	CpuSet                []int
	CpuSetStr             string
	PublicKeys            []string

	SnapshotUrl string // support for old lxc snapshot

	MemorySpecChange
}

type Provisioner interface {
	// Create a container with the supplied config
	Create(config Config) (id string, root string, err error)

	// legacy lxc support. This will enable us to create a box from backup home
	// content
	CreateFromSnapshot(config Config, aws s3.AuthInterface) (id string, root string, err error)

	// beforeStart is called after the container is mounted, but before it is
	// started so we can do more setups to make sure the container data is
	// consistent
	Start(config Config, beforeStart CallbackFunc) (id string, root string, err error)

	// Stop the container
	Stop(config Config) (id string, err error)

	// destroy the container
	Destroy(config Config) error

	ChangeConfig(config Config) error

	// get the current path to the container root
	GetRoot(name, id string) string

	// get current path to the container lxc path.
	GetLxc(name, id string) string

	// check if the current container is mounted at the correct path
	IsMounted(name, id string) (bool, error)
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
