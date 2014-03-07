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
	lxcCmd "github.com/action-io/goboxer/lxc"
	"github.com/action-io/goboxer/mount"
	pr "github.com/action-io/goboxer/provisioner"
	"github.com/action-io/goboxer/s3"
	"github.com/action-io/goboxer/utils"
	"os"
	"path"
)

const LXC_PATH = "/var/lib/lxc" // where lxc auto ecognizes the containers

func init() {
	pr.Register("lxc", Init)
}

type Provisioner struct {
	root string // root of the provisioner, typically /lxc/disk<number>
}

type lxcConfig struct {
	pr.Config
	LxcPath string
	RootFS  string
}

// Creation strategy
//
// We are going to create the delta folder, and mount it properly.
// Also populate contents of the container with template data if template is
// defined
func (p *Provisioner) Create(config pr.Config) (id string, root string, err error) {
	id, root, err = p.createBarebox(config)
	if err != nil {
		return
	}
	templatePath := path.Join(p.root, config.Template+".tar.gz")
	homePath := path.Join(root, "home", "action")
	err = archive.Untar(templatePath, homePath)
	return
}

func (p *Provisioner) CreateFromSnapshot(config pr.Config,
	aws s3.AuthInterface) (id string, root string, err error) {
	id, root, err = p.createBarebox(config)
	if err != nil {
		return
	}
	// parse snapshot url which should be an s3 path
	url, err := s3.ParseUrl(config.SnapshotUrl)
	if err != nil {
		err = &pr.OpError{Op: "parse s3 url", Err: err}
		return
	}
	client, err := aws.NewClient(url.Region, url.Bucket)
	if err != nil {
		err = &pr.OpError{Op: "create s3 client", Err: err}
		return
	}
	stream, err := client.Download(url.File)
	if err != nil {
		err = &pr.OpError{Op: "download s3 stream", Err: err}
		return
	}
	homePath := path.Join(root, "home", "action")
	if err = os.MkdirAll(homePath, 0755); err != nil {
		err = &pr.OpError{Op: "creating user home directory", Err: err}
		return
	}
	if err = archive.UntarStream(stream, homePath); err != nil {
		err = &pr.OpError{Op: "untar s3 stream", Err: err}
		return
	}
	return
}

func (p *Provisioner) Start(config pr.Config,
	beforeStart pr.CallbackFunc) (id string, root string, err error) {
	id = config.Id
	root, err = mountBox(p, config.ContainerName, config.Id)
	if err != nil {
		return
	}
	if err = p.writeConfig(config); err != nil {
		return
	}
	// trigger the callback after successfully mounted the container
	if err = beforeStart(root); err != nil {
		err = &pr.OpError{Op: "beforeStart callback " + config.ContainerName, Err: err}
		return
	}
	err = lxcCmd.Start(config.ContainerName, config.ContainerName+"-"+config.Id)
	if err != nil {
		err = &pr.OpError{Op: "lxc-start " + config.ContainerName, Err: err}
		return
	}
	return
}

func (p *Provisioner) Stop(config pr.Config) (id string, err error) {
	defer func() {
		// always generate new id
		id = generateId()
	}()
	boxId := config.ContainerName + "-" + config.Id
	if lxcCmd.Poweroff(boxId) != nil {
		utils.RunCommand("pkill", "-9", "-u", config.Uid)
		if err = lxcCmd.Stop(config.ContainerName, boxId); err != nil {
			err = &pr.OpError{Op: "lxc-stop " + boxId, Err: err}
			return
		}
	}
	err = umountBox(p, config.ContainerName, config.Id)
	return
}

// Destroy should only be called when we have stopped the box
func (p *Provisioner) Destroy(config pr.Config) error {
	boxPath := path.Join(p.root, config.ContainerName+"-"+config.Id)
	deltaPath := path.Join(p.root, config.ContainerName+"_delta")
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

func (p *Provisioner) ChangeConfig(config pr.Config) error {
	mounted, err := p.IsMounted(config.ContainerName, config.Id)
	if err != nil {
		return err
	}
	// if the box is not mounted, the next start will guarantees config change
	// so we will do an early return here
	if !mounted {
		return nil
	}
	if err := p.writeConfig(config); err != nil {
		return err
	}
	// live update the memory of the lxc container
	status, err := lxcCmd.Status(mountName(config))
	if err != nil {
		return err
	}
	if status == lxcCmd.RUNNING {
		order := config.MemorySpecChange.SettingOrder()
		for _, kvPair := range order {
			k, v := kvPair[0], kvPair[1]
			if err := lxcCmd.CgroupSet(mountName(config), k, v); err != nil {
				return err
			}
		}
	}
	return nil
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

// create a barebox that doesnt have any customized user content, but can be
// started
func (p *Provisioner) createBarebox(config pr.Config) (id string, root string, err error) {
	id = generateId()
	root, err = mountBox(p, config.ContainerName, id)
	if err != nil {
		return
	}
	config.Id = id
	err = p.writeConfig(config)
	return
}

// write lxc config into the file that will be loaded whenever lxc starts
func (p *Provisioner) writeConfig(config pr.Config) error {
	boxPath := path.Join(p.root, mountName(config))
	configFile, err := os.Create(path.Join(boxPath, "config"))
	if err != nil {
		return err
	}
	defer configFile.Close()
	return LxcTemplateCompiled.Execute(configFile, lxcConfig{
		Config:  config,
		LxcPath: boxPath,
		RootFS:  path.Join(boxPath, "rootfs"),
	})
}

// mount name of the container. Only apply for lxc container
func mountName(c pr.Config) string {
	return c.ContainerName + "-" + c.Id
}
