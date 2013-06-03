package goboxer

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

func runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	if _, err := command.CombinedOutput(); err != nil {
		return err
	}
	return nil
}

// Create delta for a box
func boxDeltaHomePath(boxer *Boxer, box *Box) string {
	return path.Join(boxer.Root, box.ContainerName()+"_delta", "rootfs",
		"home", "action")
}

func deleteLineContain(file, pattern string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	src := []string{}
	for {
		line, err := reader.ReadString('\n')
		if !strings.Contains(line, pattern) {
			src = append(src, line)
		}
		if err != nil {
			break
		}
	}
	return ioutil.WriteFile(file, []byte(strings.Join(src, "")), 0)
}

func boxPath(boxer *Boxer, box *Box) string {
	return path.Join(boxer.Root, box.ContainerName())
}

func CreateContainerPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	return os.MkdirAll(boxPath(boxer, box), 0755)
}

func DestroyContainerPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	return os.RemoveAll(boxPath(boxer, box))
}

func CreateContainerDeltaPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	return os.MkdirAll(boxDeltaHomePath(boxer, box), 0755)
}

func DestroyContainerDeltaPath(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	target := path.Join(boxer.Root, box.ContainerName()+"_delta")
	for retries := 0; retries < 1000; retries++ {
		err := os.RemoveAll(target)
		if err == nil {
			// rm mntpoint succeeded
			return nil
		}
		if os.IsNotExist(err) {
			// mntpoint doesn't exist anymore. Success.
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("Failed to remove % v", target)
}

// mount the container
func MountContainer(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	boxDeltaPath := path.Join(boxer.Root, box.ContainerName()+"_delta")
	boxPath := path.Join(boxer.Root, box.ContainerName())
	baseContainerPath := path.Join(boxer.Root, "base_container_template")
	return MountOverlay(boxPath, boxDeltaPath, baseContainerPath)
}

func UnmountContainer(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	boxPath := path.Join(boxer.Root, box.ContainerName())
	return UnmountOverlay(boxPath)
}

// untar the template
func ExtractTemplate(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join("/var/lib/lxc", box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	homeDataPath := path.Join("/var/lib/lxc", box.TemplatePath())
	return Untar(homeDataPath, boxDeltaHomePath)
}

// set /etc/projects
func SetEtcProjects(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join("/var/lib/lxc", box.ContainerName()+"_delta")
	boxDeltaHomePath := path.Join(boxDeltaPath, "rootfs", "home", "action")
	projectsFile, err := os.OpenFile("/etc/projects", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer projectsFile.Close()
	projectLine := box.Id + ":" + boxDeltaHomePath
	_, err = projectsFile.Write([]byte(projectLine))
	return err
}

func UnsetEtcProjects(c *BoxCommand) error {
	box := c.Box
	boxDeltaPath := path.Join("/var/lib/lxc", box.ContainerName()+"_delta")
	return deleteLineContain("/etc/projects", boxDeltaPath)
}

// set /etc/projid
func SetEtcProjid(c *BoxCommand) error {
	box := c.Box
	projidFile, err := os.OpenFile("/etc/projid", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer projidFile.Close()
	projidLine := box.ContainerName() + ":" + box.Id
	_, err = projidFile.Write([]byte(projidLine))
	return err
}
func UnsetEtcProjid(c *BoxCommand) error {
	box := c.Box
	projidLine := box.ContainerName() + ":" + box.Id
	return deleteLineContain("/etc/projid", projidLine)
}

// set xfs_quota
func SetXfsQuota(c *BoxCommand) error {
	box := c.Box
	runCommand("xfs_quota", "-x", "-c", "project -s "+box.ContainerName(),
		"/var/lib/lxc")
	runCommand("xfs_quota", "-x", "-c",
		"limit -p bhard="+box.DiskSpace+" "+box.ContainerName(),
		"/var/lib/lxc")
	return nil
}

func UnsetXfsQuota(c *BoxCommand) error {
	box := c.Box
	runCommand("xfs_quota", "-x", "-c",
		"\"limit -p bhard=0 "+box.ContainerName()+"\"", "/var/lib/lxc")
	return nil
}

// set lxc_config
func SetLxcConfig(c *BoxCommand) error {
	boxer := c.Boxer
	box := c.Box
	lxcConfigPath := path.Join(boxer.Root, box.ContainerName(), "config")
	lxcConfig, err := os.Create(lxcConfigPath)
	if err != nil {
		return err
	}
	defer lxcConfig.Close()
	return LxcTemplateCompiled.Execute(lxcConfig, box)
}

// set hostname
func SetHostname(c *BoxCommand) error {
	box := c.Box
	hostNamePath := path.Join(box.RootFS(), "etc", "hostname")
	hostName, err := os.Create(hostNamePath)
	if err != nil {
		return err
	}
	defer hostName.Close()
	_, err = hostName.Write([]byte(box.ContainerName()))
	return err
}

// set network interface
func SetNetworkConfig(c *BoxCommand) error {
	box := c.Box
	networkConfigPath := path.Join(box.RootFS(), "etc", "network", "interfaces")
	networkConfig, err := os.Create(networkConfigPath)
	if err != nil {
		return err
	}
	defer networkConfig.Close()
	return NetworkConfigCompiled.Execute(networkConfig, box)
}

// set uid
func SetUserUid(c *BoxCommand) error {
	box := c.Box
	return runCommand("chroot", box.RootFS(), "usermod", "-u", box.Uid(), "action")
}

func SetUserGid(c *BoxCommand) error {
	box := c.Box
	return runCommand("chroot", box.RootFS(), "groupmod", "-g", box.Gid(), "action")
}

// set gid
func SetHomePermission(c *BoxCommand) error {
	box := c.Box
	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	return runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action")
}

// create workspace
func SetWorkspace(c *BoxCommand) error {
	box := c.Box
	if err := os.MkdirAll(path.Join(box.RootFS(), "home", "action",
		"workspace"), 0755); err != nil {
		return err
	}
	permission := fmt.Sprintf("%s:%s", box.Uid(), box.Gid())
	return runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action/workspace")
}

// set /etc/action/box_id
func SetBoxId(c *BoxCommand) error {
	box := c.Box
	boxIdFile, err := os.Create(box.BoxIdFile())
	if err != nil {
		return err
	}
	defer boxIdFile.Close()
	_, err = boxIdFile.Write([]byte(box.Id))
	return err
}

func UnsetBoxId(c *BoxCommand) error {
	return os.RemoveAll(c.Box.BoxIdFile())
}

// setup port forwarding
func SetPortForwarding(c *BoxCommand) error {
	box := c.Box
	runCommand("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp",
		"--dport", box.Port, "-j", "DNAT", "--to",
		box.Ip+":22")
	return nil
}

func UnsetPortForwarding(c *BoxCommand) error {
	box := c.Box
	runCommand("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp",
		"--dport", box.Port, "-j", "DNAT", "--to", box.Ip+":22")
	return nil
}

// setup default key pair
func SetDefaultKeyPair(c *BoxCommand) error {
	box := c.Box
	// chroot and run ssh-keygen
	sshPath := path.Join(c.Box.RootFS(), "home", "action", ".ssh")
	sshKeyFile := path.Join(c.Box.RootFS(), c.Box.PrivateKeyPath())
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
	return runCommand("chroot", box.RootFS(), "chown", "-R", permission,
		"/home/action/.ssh")
}

// Stop Container
func StopContainer(c *BoxCommand) error {
	box := c.Box
	err := runCommand("lxc-stop", "-n", box.ContainerName())
	if err != nil {
		return err
	}
	return runCommand("lxc-wait", "-n", box.ContainerName(), "-s", "STOPPED")
}

func StartContainer(c *BoxCommand) error {
	box := c.Box
	err := runCommand("lxc-start", "-d", "-n", box.ContainerName())
	if err != nil {
		return err
	}
	return runCommand("lxc-wait", "-n", box.ContainerName(), "-s", "RUNNING")
}

// Default functionw when you want to continue over to the next state
func Pass(c *BoxCommand) error {
	return nil
}
