// lxc provisioner
//
// This is the provisioner based on lxc and aufs. This assumes a certain
// structure on how we setup the root disk such as
// - base_container_template
// - various templates such as ruby_template.tar.gz, etc...
//
// The result container will be mounted at LXC_PATH when start to make sure
// that lxc can automatically recognize them
package lxc

import (
	"github.com/action-io/goboxer/archive"
	"github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	pr "github.com/action-io/goboxer/provisioner"
	"github.com/action-io/goboxer/s3"
	"github.com/action-io/goboxer/utils"
	"os"
	"path"
	"strconv"
	"time"
)

const LXC_PATH = "/var/lib/lxc" // where lxc auto ecognizes the containers

func init() {
	pr.Register("lxc", Init)
}

type Provisioner struct {
	root string // root of the provisioner, typically /lxc/disk<number>
}

// Creation strategy
//
// We are going to create the delta folder, and mount it properly.
// Also populate contents of the container with template data if template is
// defined
func (p *Provisioner) Create(name, template string) (*pr.Data, error) {
	data := &pr.Data{
		Name:      name,
		Id:        generateId(),
		DeltaPath: path.Join(p.root, name+"_delta"),
		Status:    "stopped",
	}
	err := mountBox(p.root, data)
	if err != nil {
		return data, err
	}
	templatePath := path.Join(p.root, template+".tar.gz")
	homePath := path.Join(data.RootPath, "home", "action")
	if err := archive.Untar(templatePath, homePath); err != nil {
		return data, err
	}
	return data, nil
}

func (p *Provisioner) CreateFromSnapshot(name, snapshotUrl string,
	aws s3.AuthInterface) (*pr.Data, error) {
	data := &pr.Data{
		Name:      name,
		Id:        generateId(),
		DeltaPath: path.Join(p.root, name+"_delta"),
		Status:    "stopped",
	}
	err := mountBox(p.root, data)
	if err != nil {
		return data, err
	}
	// parse snapshot url which should be an s3 path
	url, err := s3.ParseUrl(snapshotUrl)
	if err != nil {
		return data, &pr.OpError{Op: "parse s3 url", Err: err}
	}
	client, err := aws.NewClient(url.Region, url.Bucket)
	if err != nil {
		return data, &pr.OpError{Op: "create s3 client", Err: err}
	}
	stream, err := client.Download(url.File)
	if err != nil {
		return data, &pr.OpError{Op: "download s3 stream", Err: err}
	}
	homePath := path.Join(data.RootPath, "home", "action")
	if err := os.MkdirAll(homePath, 0755); err != nil {
		return data, &pr.OpError{Op: "creating user home directory", Err: err}
	}
	if err := archive.UntarStream(stream, homePath); err != nil {
		return data, &pr.OpError{Op: "untar s3 stream", Err: err}
	}
	return data, nil
}

func (p *Provisioner) Start(name, id string, beforeStart pr.CallbackFunc) (*pr.Data, error) {
	data := &pr.Data{
		Name:      name,
		Id:        id,
		DeltaPath: path.Join(p.root, name+"_delta"),
	}
	if err := mountBox(p.root, data); err != nil {
		return data, err
	}
	// trigger the callback after successfully mounted the container
	if err := beforeStart(data); err != nil {
		return data, &pr.OpError{Op: "beforeStart callback " + data.Name, Err: err}
	}
	err := lxc.Start(data.Name, data.Name+"-"+data.Id)
	if err != nil {
		return data, &pr.OpError{Op: "lxc-start " + data.Name, Err: err}
	}
	data.Status = "running"
	return data, nil
}

func (p *Provisioner) Stop(name, id, uid string) (*pr.Data, error) {
	data := &pr.Data{
		Name:      name,
		Id:        id,
		DeltaPath: path.Join(p.root, name+"_delta"),
	}
	boxId := name + "-" + id
	if lxc.Poweroff(boxId) != nil {
		utils.RunCommand("pkill", "-9", "-u", uid)
		if err := lxc.Stop(name, boxId); err != nil {
			return data, &pr.OpError{Op: "lxc-stop " + boxId, Err: err}
		}
	}
	data.Status = "stopped"
	return umountBox(p.root, data)
}

// Destroy should only be called when we have stopped the box
func (p *Provisioner) Destroy(name, id string) error {
	boxPath := path.Join(p.root, name+"-"+id)
	deltaPath := path.Join(p.root, name+"_delta")
	// if golang failed to remove container, drops into cmdline as it seems to
	// work all the times. We just prefer to do it in golang first.
	if err := os.RemoveAll(deltaPath); err != nil {
		err = utils.RunCommand("rm", "-r", "-f", deltaPath)
		if err != nil {
			return &pr.OpError{Op: "remove container delta", Err: err}
		}
	}
	err := os.RemoveAll(boxPath)
	if err != nil {
		return &pr.OpError{Op: "remove container FS", Err: err}
	}
	return nil
}

func (p *Provisioner) SetConfig(name, id string, f func(config *os.File) error) error {
	boxPath := path.Join(p.root, name+"-"+id)
	config, err := os.Create(path.Join(boxPath, "config"))
	if err != nil {
		return err
	}
	defer config.Close()
	return f(config)
}

func (p *Provisioner) GetLxc(name, id string) string {
	return path.Join(p.root, name+"-"+id)
}

func (p *Provisioner) GetRoot(name, id string) string {
	return path.Join(p.root, name+"-"+id, "rootfs")
}

func (p *Provisioner) IsMounted(name, id string) (bool, error) {
	return mount.Mounted(path.Join(p.root, name+"-"+id))
}

func Init(root string) (pr.Provisioner, error) {
	return &Provisioner{root: root}, nil
}

func generateId() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// populate data.RootPath if successfully mount the box
func mountBox(root string, data *pr.Data) error {
	boxId := data.Name + "-" + data.Id
	boxPath := path.Join(root, boxId)
	baseContainerPath := path.Join(root, "base_container_template")
	lxcPath := path.Join(LXC_PATH, boxId)
	if err := mount.Union("aufs", boxPath, data.DeltaPath, baseContainerPath); err != nil {
		return &pr.OpError{Op: "lxc: overlayfs/aufs mount", Err: err}
	}
	// FIXME: MountBind to make it similar to boxer. Ideally we probably
	// want to symlink as it doesnt make any difference and seems to be
	// more stable. Less mountpoint is better
	err := mount.MountBind(boxPath, lxcPath)
	if err != nil {
		return &pr.OpError{Op: "lxc: overlayfs/aufs mount-bind", Err: err}
	}
	data.RootPath = path.Join(lxcPath, "rootfs")
	return nil
}

func umountBox(root string, data *pr.Data) (*pr.Data, error) {
	boxId := data.Name + "-" + data.Id
	boxPath := path.Join(root, boxId)
	lxcPath := path.Join(LXC_PATH, boxId)
	// unmount both /var/lib/lxc/mclovins-container-1-1234 and
	// /lxc/disk8/mclovins-container-1-1234
	// but ignore errors here because they are not important for the next start
	// since we always re-generate the mountpoints
	mount.Unmount("aufs", lxcPath)
	mount.Unmount("aufs", boxPath)
	return data, nil
}
