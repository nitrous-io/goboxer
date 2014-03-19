// lxc provisioner
//
// This is the provisioner based on lxc and aufs. This assumes a certain
// structure on how we setup the root disk such as
// - base_container_template
// - delta folder
// - union with aufs
//
// The result container will be mounted at LXC_PATH when start to make sure
// that lxc can automatically recognize them
package lxc

import (
	"os"
	"path"
	"strconv"
	"time"

	lxcCmd "github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	pr "github.com/action-io/goboxer/provisioner"
	"github.com/action-io/goboxer/utils"
)

const LXC_PATH = "/var/lib/lxc" // where lxc auto ecognizes the containers
const BASE = "base_container_template"

func init() {
	pr.Register("lxc", Init)
}

type Provisioner struct {
	root string // root of the provisioner, typically /lxc/disk<number>
}

// create basic structure of the container. This doesn't handle container's
// data and config yet.
func (p *Provisioner) Create(name string) (string, error) {
	id := generateId()
	if err := p.mount(name, id); err != nil {
		return id, err
	}
	return id, nil
}

func (p *Provisioner) Destroy(name, id string) error {
	deltaPath := path.Join(p.root, name+"_delta")
	// if golang failed to remove container, drops into cmdline as it seems to
	// work all the times. We just prefer to do it in golang first.
	if err := os.RemoveAll(deltaPath); err != nil {
		err = utils.RunCommand("rm", "-r", "-f", deltaPath)
		if err != nil {
			return &pr.OpError{Op: "remove container delta", Err: err}
		}
	}
	return nil
}

func (p *Provisioner) Stop(name, boxId, userId string) (id string, err error) {
	defer func() {
		// always generate new id
		id = generateId()
	}()
	containerName := name + "-" + boxId
	if lxcCmd.Poweroff(p.GetCurrentRoot(name, boxId), containerName) != nil {
		utils.RunCommand("pkill", "-9", "-u", userId)
		if err = lxcCmd.Stop(p.GetCurrentRoot(name, boxId), name, containerName); err != nil {
			err = &pr.OpError{Op: "lxc-stop " + boxId, Err: err}
			return
		}
	}
	err = p.unmount(name, boxId)
	return
}

func (p *Provisioner) Start(name, boxId string, beforeStart func() error) (string, error) {
	if err := p.mount(name, boxId); err != nil {
		return boxId, err
	}
	if err := beforeStart(); err != nil {
		err = &pr.OpError{Op: "beforeStart callback " + name, Err: err}
		return boxId, err
	}
	if err := lxcCmd.Start(p.root, name, name+"-"+boxId); err != nil {
		err = &pr.OpError{Op: "lxc-start " + name, Err: err}
		return boxId, err
	}
	return boxId, nil
}

func (p *Provisioner) GetLxcPath(name, id string) string {
	return path.Join(p.GetCurrentRoot(name, id), name+"-"+id)
}

func (p *Provisioner) GetLxcConfigPath(name, id string) string {
	return path.Join(p.GetCurrentRoot(name, id), name+"-"+id, "config")
}

func (p *Provisioner) GetRootfsPath(name, id string) string {
	return path.Join(p.GetCurrentRoot(name, id), name+"-"+id, "rootfs")
}

func (p *Provisioner) IsMounted(name, id string) (bool, error) {
	return mount.Mounted(path.Join(p.GetCurrentRoot(name, id), name+"-"+id))
}

func Init(root string) (pr.Provisioner, error) {
	return &Provisioner{root: root}, nil
}

func generateId() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// populate data.RootPath if successfully mount the box
func (p *Provisioner) mount(name, id string) error {
	baseContainerPath := path.Join(p.root, BASE)
	deltaPath := path.Join(p.root, name+"_delta")
	boxId := name + "-" + id
	boxPath := path.Join(p.root, boxId)
	if err := mount.Union("aufs", boxPath, deltaPath, baseContainerPath); err != nil {
		return &pr.OpError{Op: "lxc: overlayfs/aufs mount", Err: err}
	}
	return nil
}

func (p *Provisioner) unmount(name, id string) error {
	boxId := name + "-" + id
	boxPath := path.Join(p.root, boxId)
	oldLxcPath := path.Join(LXC_PATH, boxId)
	// unmount /lxc/disk8/mclovins-container-1-1234
	// but ignore errors here because they are not important for the next start
	// since we always re-generate the mountpoints
	mount.Unmount("aufs", oldLxcPath)
	mount.Unmount("aufs", boxPath)
	return nil
}

// detects the correct container path of the box ( which can be either b.Disk
// or /var/lib/lxc ). This is used for the migration process where container
// can be in /var/lib/lxc when we deploy new version of boxer, hence will not
// work with the new lxc path
func (p *Provisioner) GetCurrentRoot(name, id string) string {
	mountName := name + "-" + id
	// we check the old lxc path to see if this container is still using it
	// if the mount point is not exist, then we can safely use the new disk
	oldLxcPath := path.Join("/var/lib/lxc", mountName)
	if _, err := os.Stat(oldLxcPath); os.IsNotExist(err) {
		return p.root
	}
	// otherwise, check if it's mounted to make sure that its in used
	// if there are errors, conservatively use the old path
	// if this is mounted, a container is using it.
	mounted, err := mount.Mounted(oldLxcPath)
	if err != nil || mounted {
		return "/var/lib/lxc"
	}
	// not mounted so we can safely use the new disk since container are not
	// used
	return p.root
}
