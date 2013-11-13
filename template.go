package goboxer

import (
	"text/template"
)

const LxcTemplate = `
# uncomment the next line to run the container unconfined:
#lxc.aa_profile = unconfined
lxc.devttydir = lxc
lxc.tty = 4
lxc.pts = 1024
lxc.arch = amd64
lxc.pivotdir = lxc_putold

lxc.cap.drop = audit_control audit_write
lxc.cap.drop = mac_admin mac_override
lxc.cap.drop = mknod setfcap setpcap
lxc.cap.drop = sys_boot sys_module sys_nice sys_pacct sys_rawio sys_resource
lxc.cap.drop = sys_time sys_tty_config

lxc.network.type = veth
lxc.network.flags = up
lxc.network.link = {{.NetworkInterface}}
lxc.network.hwaddr = {{.MacAddress}}

lxc.cgroup.memory.limit_in_bytes = {{.Memory}}
lxc.cgroup.memory.memsw.limit_in_bytes = {{.TotalMemory}}
lxc.cgroup.memory.swappiness = {{.Swappiness}}
lxc.cgroup.cpu.shares = {{.CpuShares}}
lxc.cgroup.devices.deny = a
# Allow any mknod (but not using the node)
lxc.cgroup.devices.allow = c *:* m
lxc.cgroup.devices.allow = b *:* m
# /dev/null and zero
lxc.cgroup.devices.allow = c 1:3 rwm
lxc.cgroup.devices.allow = c 1:5 rwm
# consoles
lxc.cgroup.devices.allow = c 5:1 rwm
lxc.cgroup.devices.allow = c 5:0 rwm
#lxc.cgroup.devices.allow = c 4:0 rwm
#lxc.cgroup.devices.allow = c 4:1 rwm
# /dev/{,u}random
lxc.cgroup.devices.allow = c 1:9 rwm
lxc.cgroup.devices.allow = c 1:8 rwm
lxc.cgroup.devices.allow = c 136:* rwm
lxc.cgroup.devices.allow = c 5:2 rwm
# rtc
lxc.cgroup.devices.allow = c 254:0 rwm
#fuse
lxc.cgroup.devices.allow = c 10:229 rwm
#tun
lxc.cgroup.devices.allow = c 10:200 rwm
#full
lxc.cgroup.devices.allow = c 1:7 rwm
#hpet
lxc.cgroup.devices.allow = c 10:228 rwm
#kvm
lxc.cgroup.devices.allow = c 10:232 rwm

lxc.utsname = {{.ContainerName}}

{{$LXCPATH := .LxcPath}}
{{$ROOTFS := .RootFS}}
lxc.rootfs = {{$ROOTFS}}
lxc.mount  = {{$LXCPATH}}/fstab
`
const NetworkConfigTemplate = `
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address {{.Ip}}
    netmask 255.255.255.0
    gateway {{.Gateway}}
`

const HostsTemplate = `
127.0.0.1 localhost
127.0.1.1 {{.ContainerName}}
# The following lines are desirable for IPv6 capable hosts
::1 ip6-localhost ip6-loopback
fe00::0 ip6-localnet
ff00::0 ip6-mcastprefix
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
`

var NetworkConfigCompiled *template.Template
var LxcTemplateCompiled *template.Template
var HostsTemplateCompiled *template.Template

func init() {
	var err error
	HostsTemplateCompiled, err = template.New("hosts").Parse(HostsTemplate)
	if err != nil {
		panic(err)
	}
	NetworkConfigCompiled, err = template.New("network").Parse(NetworkConfigTemplate)
	if err != nil {
		panic(err)
	}
	LxcTemplateCompiled, err = template.New("lxc").Parse(LxcTemplate)
	if err != nil {
		panic(err)
	}
}
