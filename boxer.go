// Similar to docker runtime. This manages a list of containers
//
// We differentiate from docker in:
// - using `ResourcePool` to manages available resources i.e ip, open port etc
// instead of auto-generating them on the fly
package goboxer

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
)

type Boxer struct {
	root     string
	runtimes map[string]string
}

func (boxer *Boxer) Create(config *Config, resource *Resource) (*Box, error) {
	var box *Box
	var err error
	box = &Box{Config: config, Resource: resource}
	err = setupContainerDirectory(box)
	err = mountContainer(box)
	err = setupQuota(box)
	err = configureContainer(box)
	err = setPortForwarding(box)
	err = setDefaultKeyPair(box)
	// err :_ = saveContainer(boxer.redisClient, box)
	return box, err
}

func runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	if output, err := command.CombinedOutput(); err != nil {
		debug("cmd", command)
		debug("output", output)
		debug("error", err)
		return err
	}
	return nil
}

func setupContainerDirectory(box *Box) error {
	boxPath := path.Join("/var/lib/lxc", box.ContainerName())
	boxDeltaPath := path.Join("/var/lib/lxc", box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	if err := os.MkdirAll(boxPath, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(boxDeltaHomePath, 0755); err != nil {
		return err
	}
	return nil
}

func mountContainer(box *Box) error {
	// TODO: lock this
	boxDeltaPath := path.Join("/var/lib/lxc", box.ContainerName()+"_delta")
	boxPath := path.Join("/var/lib/lxc", box.ContainerName())
	baseContainerPath := path.Join("/var/lib/lxc", "base_container_template")

	// mount overlay
	if err := MountOverlay(boxPath, boxDeltaPath,
		baseContainerPath); err != nil {
		return err
	}
	return nil
}

func setupQuota(box *Box) error {
	boxDeltaPath := path.Join("/var/lib/lxc", box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	// set /etc/projects
	projectsFile, err := os.OpenFile("/etc/projects", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer projectsFile.Close()
	if _, err = projectsFile.Write([]byte(strconv.Itoa(box.Id()) + ":" +
		boxDeltaHomePath)); err != nil {
		return err
	}

	// set /etc/projid
	projidFile, err := os.OpenFile("/etc/projid", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer projidFile.Close()
	if _, err = projidFile.Write([]byte(box.ContainerName() + ":" +
		strconv.Itoa(box.Id()))); err != nil {
		return err
	}

	// set xfs quota and ignore its errors
	_ = runCommand("xfs_quota", "-x", "-c", "project -s "+box.ContainerName(),
		"/var/lib/lxc")
	_ = runCommand("xfs_quota", "-x", "-c",
		"limit -p bhard="+box.DiskSpace()+" "+box.ContainerName(),
		"/var/lib/lxc")
	return err
}

func configureContainer(box *Box) error {
	homeDataPath := path.Join("/var/lib/lxc", box.Template())
	boxDeltaPath := path.Join("var/lib/lxc", box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")

	// untar the template
	Untar(homeDataPath, boxDeltaHomePath)

	// set lxc config
	lxcConfigPath := path.Join("/var/lib/lxc", box.ContainerName(), "config")
	lxcConfig, err := os.Create(lxcConfigPath)
	if err != nil {
		return err
	}
	defer lxcConfig.Close()
	if err := LxcTemplateCompiled.Execute(lxcConfig, box); err != nil {
		return err
	}

	// set hostname
	hostNamePath := path.Join(box.RootFS(), "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	if _, err = hostName.Write([]byte(box.Hostname())); err != nil {
		return err
	}

	// set network interfaces
	networkConfigPath := path.Join(box.RootFS(), "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	if err := NetworkConfigCompiled.Execute(networkConfig, box); err != nil {
		return err
	}

	// set uid
	if err = runCommand("chroot", box.RootFS(), "usermod", "-u",
		box.Uid(), "action"); err != nil {
		return err
	}

	//// set gid
	if err = runCommand("chroot", box.RootFS(), "groupmod", "-g",
		box.Gid(), "action"); err != nil {
		return err
	}

	//// Set uid/gid for home
	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	if err = runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action"); err != nil {
		return err
	}

	// create workspace
	if err = os.MkdirAll(path.Join(box.RootFS(), "home", "action",
		"workspace"), 0755); err != nil {
		return err
	}
	if err = runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action/workspace"); err != nil {
		return err
	}

	// set /etc/action/box_id
	boxIdFile, err := os.Create(box.BoxIdFile())
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	if _, err = boxIdFile.Write([]byte(strconv.Itoa(box.Id()))); err != nil {
		return err
	}
	return nil
}

func setPortForwarding(box *Box) error {
	// TODO: lock this
	if err := runCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", box.Port(), "-j", "DNAT", "--to",
		box.IpAddress()+":22"); err != nil {
		return err
	}
	return nil
}

func setDefaultKeyPair(box *Box) error {
	// chroot and run ssh-keygen
	sshPath := path.Join(box.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(box.RootFS(), box.PrivateKeyPath())
	if err := os.MkdirAll(sshPath, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(sshKeyFile); os.IsNotExist(err) {
		sshKeygen := fmt.Sprintf("ssh-keygen -N '' -f %s -b 1024 -t rsa -q",
			sshKeyFile)
		if err := runCommand("/bin/sh", "-c", sshKeygen); err != nil {
			return err
		}
	} else {
		return err
	}

	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	if err := runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action/.ssh"); err != nil {
		return err
	}
	return nil
}
