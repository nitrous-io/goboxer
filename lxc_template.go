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
lxc.cap.drop = sys_admin

lxc.network.type = veth
lxc.network.flags = up
lxc.network.link = {{.NetworkInterface}}
lxc.network.hwaddr = {{.MacAddress}}

lxc.cgroup.memory.limit_in_bytes = {{.Memory}}
lxc.cgroup.memory.memsw.limit_in_bytes = {{.TotalMemory}}
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

lxc.utsname = {{.Hostname}}


{{$CONTAINERFS := .ContainerFS }}
{{$ROOTFS := .RootFS}}
lxc.rootfs = {{$ROOTFS}}
lxc.mount  = {{$CONTAINERFS}}/fstab
lxc.mount.entry = /sys/fs/fuse/connections /sys/fs/fuse/connections fusectl optional 0 0
lxc.mount.entry = /sys/kernel/debug /sys/kernel/debug debugfs optional 0 0
lxc.mount.entry = /sys/kernel/security {{$ROOTFS}}/sys/kernel/security securityfs optional 0 0
lxc.mount.entry = /run {{$ROOTFS}}/run tmpfs noexec,nosuid,size=10%,mode=0755  0 0
`

var LxcTemplateCompiled *template.Template

func init() {
	var err error
	LxcTemplateCompiled, err = template.New("lxc").Parse(LxcTemplate)
	if err != nil {
		panic(err)
	}
}
