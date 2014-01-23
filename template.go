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
lxc.cap.drop = sys_module sys_nice sys_pacct sys_rawio sys_resource
lxc.cap.drop = sys_time sys_tty_config

lxc.network.type = veth
lxc.network.flags = up
lxc.network.link = {{.NetworkInterface}}
lxc.network.hwaddr = {{.MacAddress}}

lxc.cgroup.memory.limit_in_bytes = {{.Memory}}
lxc.cgroup.memory.memsw.limit_in_bytes = {{.TotalMemory}}
lxc.cgroup.memory.swappiness = {{.Swappiness}}
lxc.cgroup.cpu.shares = {{.CpuShares}}
{{if .CpuQuota}}
lxc.cgroup.cpu.cfs_period_us = {{.CpuPeriodMicroseconds}}
lxc.cgroup.cpu.cfs_quota_us = {{.CpuQuotaMicroseconds}}
{{end}}
{{if .CpuSet}}
lxc.cgroup.cpuset.cpus = {{.CpuSetStr}}
{{end}}
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
lxc.mount.entry = proc proc proc nodev,noexec,nosuid 0 0
lxc.mount.entry = sysfs sys sysfs defaults 0 0
lxc.mount.entry = /sys/fs/fuse/connections sys/fs/fuse/connections none bind,optional 0 0
lxc.mount.entry = /sys/kernel/debug sys/kernel/debug none bind 0 0
lxc.mount.entry = /sys/kernel/security sys/kernel/security none bind 0 0

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

const ReadmeTemplate = `
# Welcome to Nitrous.IO

Nitrous.IO enables you to develop web applications completely in the
cloud. This development "box" helps you write software, collaborate
real-time with friends, show off apps to teammates or clients, and
deploy apps to production hosting sites like Heroku or Google App
Engine.

## Getting Started

This box is a fully functional Linux environment in which you can
develop any Linux-based application. This box comes bundled with gcc,
make, perl and other system-level libraries, enough to get you started
on your application development journey.


## Setting up your SSH Keys

We recommend that you use Github (www.github.com) to manage your
application's code. To interact with your code on Github, you'll need to
add your Nitrous.IO box's SSH keys to Github.  Follow these steps to get
started:

http://help.nitrous.io/github-add-key/

## Installing Databases

Your box comes installed with Autoparts, with which you can install
databases such as MySQL, Redis, Postgres, Memcache and many others.
Look for the "Autoparts" menu in the IDE, or read more here:

http://help.nitrous.io/autoparts/

## Previewing your application

Once you're running a webserver, keep an eye on the port where your
server is running.  Then click the "Preview" menu title in the IDE above
and select the port where your application is running. For more
information, check out this help article:

http://help.nitrous.io/preview/

## Real-Time Collaboration

You can invite friends and teammates into this web IDE session so you
can code collaboratively on the same codebase.  This can be really
helpful when giving tutorials, troubleshooting some code, or just doing
some pair programming.  Click the "Collaborate" menu and then manage
collaborators.

http://help.nitrous.io/collab/

## Deployment

Once you've built your application, you can deploy to various
cloud services such as Heroku, Google App Engine, Nodejitsu, Azure and
others.  You can read more about it here:

http://help.nitrous.io/categories/deployment/

{{template "runtime"}}
`
const TestTemplate = `
{{define "runtime"}}
## Test Template

This is the test template
{{end}}
`

const RailsTemplate = `
{{define "runtime"}}
## Ruby / Rails

Ruby on this box is managed using [RVM](http://rvm.io) . You can install
other Ruby versions by using the rvm command. Read more about the rvm
command here:

http://rvm.io/rubies/installing

This Ruby box also comes installed with Node.JS which is managed using
[nvm](https://github.com/creationix/nvm). You can read more about how
you can upgrade/downgrade node.js here:

https://github.com/creationix/nvm#usage
{{end}}
`

const DjangoTemplate = `
{{template "runtime"}}
## Python

This box comes installed with virtualenv and you can use other versions
of Python. Read more about it here:

http://help.nitrous.io/python-versions-with-virtualenv/

You can also read more about creating a Django app here:

http://help.nitrous.io/django-app/
{{end}}
`

const NodejsTemplate = `
{{template "runtime"}}
## Node.JS

Node.JS on this box is managed using
[nvm](https://github.com/creationix/nvm). You can read more about how
you can upgrade/downgrade node.js here:

https://github.com/creationix/nvm#usage

You can read more about running a static site here:

http://help.nitrous.io/node-connect-server/

Or deploying to Nodejitsu here:

http://help.nitrous.io/nodejitsu/
{{end}}
`

const GoTemplate = `
{{template "runtime"}}
## Go

The Go binary is installed here:

/home/action/.go/bin/go
{{end}}
`

var NetworkConfigCompiled *template.Template
var LxcTemplateCompiled *template.Template
var HostsTemplateCompiled *template.Template
var ReadmeTemplateCompiled *template.Template

var RuntimeTemplates map[string]string = map[string]string{
	"test":   TestTemplate,
	"rails":  RailsTemplate,
	"django": DjangoTemplate,
	"nodejs": NodejsTemplate,
	"go":     GoTemplate,
}

func init() {
	HostsTemplateCompiled = template.Must(template.New("hosts").Parse(HostsTemplate))
	NetworkConfigCompiled = template.Must(template.New("network").Parse(NetworkConfigTemplate))
	LxcTemplateCompiled = template.Must(template.New("lxc").Parse(LxcTemplate))
	ReadmeTemplateCompiled = template.Must(template.New("readme").Parse(ReadmeTemplate))
}
