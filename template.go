package goboxer

import (
	"fmt"
	"text/template"
)

const LxcTemplate = `
{{if .Sudo}}
# To support container nesting on an Ubuntu host while retaining most of
# apparmor's added security, use the following two lines instead.
# lxc.aa_profile = lxc-container-default-with-nesting
lxc.aa_profile = unconfined
lxc.hook.mount = /usr/share/lxc/hooks/mountcgroups
{{else}}
# uncomment the next line to run the container unconfined
# lxc.aa_profile = unconfined
{{end}}

lxc.devttydir = lxc
lxc.tty = 4
lxc.pts = 1024
lxc.arch = amd64
lxc.pivotdir = lxc_putold

{{if .Sudo}}
lxc.cap.drop = sys_module mac_admin mac_override sys_time
{{else}}
lxc.cap.drop = audit_control audit_write
lxc.cap.drop = mac_admin mac_override
lxc.cap.drop = mknod setfcap setpcap
lxc.cap.drop = sys_module sys_nice sys_pacct sys_rawio sys_resource
lxc.cap.drop = sys_time sys_tty_config
{{end}}

lxc.network.type = veth
lxc.network.flags = up
lxc.network.link = {{.NetworkInterface}}
lxc.network.hwaddr = {{.MacAddress}}

lxc.cgroup.memory.limit_in_bytes = {{.Memory}}
lxc.cgroup.memory.memsw.limit_in_bytes = {{.TotalMemory}}
lxc.cgroup.cpu.shares = {{.CpuShares}}
{{if .CpuQuota}}
lxc.cgroup.cpu.cfs_period_us = {{.CpuPeriodMicroseconds}}
lxc.cgroup.cpu.cfs_quota_us = {{.CpuQuotaMicroseconds}}
{{end}}
{{if .CpuSet}}
lxc.cgroup.cpuset.cpus = {{.CpuSetStr}}
{{end}}

{{if .Sudo}}
# allow all devices in sudo mode
lxc.cgroup.devices.allow = a
{{else}}
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
{{end}}

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

const ReadmeTemplate = `# Welcome to Nitrous.IO

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
`

var TestTemplate = fmt.Sprintf(`
%s
## Test Template

This is the test template
`, ReadmeTemplate)

var RailsTemplate = fmt.Sprintf(`%s
## Ruby / Rails

Ruby on this box is managed using [RVM](http://rvm.io) . You can install
other Ruby versions by using the rvm command. Read more about the rvm
command here:

http://rvm.io/rubies/installing

This Ruby box also comes installed with Node.JS which is managed using
[nvm](https://github.com/creationix/nvm). You can read more about how
you can upgrade/downgrade node.js here:

https://github.com/creationix/nvm#usage
`, ReadmeTemplate)

var DjangoTemplate = fmt.Sprintf(`%s
## Python

This box comes installed with virtualenv and you can use other versions
of Python. Read more about it here:

http://help.nitrous.io/python-versions-with-virtualenv/

You can also read more about creating a Django app here:

http://help.nitrous.io/django-app/
`, ReadmeTemplate)

var NodejsTemplate = fmt.Sprintf(`%s
## Node.JS

Node.JS on this box is managed using
[nvm](https://github.com/creationix/nvm). You can read more about how
you can upgrade/downgrade node.js here:

https://github.com/creationix/nvm#usage

You can read more about running a static site here:

http://help.nitrous.io/node-connect-server/

Or deploying to Nodejitsu here:

http://help.nitrous.io/nodejitsu/
`, ReadmeTemplate)

var GoTemplate = fmt.Sprintf(`%s
## Go

The Go binary is installed here:

/home/action/.go/bin/go
`, ReadmeTemplate)

var PhpTemplate = fmt.Sprintf(`%s
## PHP

This box comes installed with Apache2, MySQL, Composer and PHP. Apache is running on Port 3000
by default, so you can start previewing your PHP applications by clicking "Preview" and clicking
"Port 3000". You can read more about PHP here:

http://help.nitrous.io/php/
`, ReadmeTemplate)

var UpstartConfSettings = map[string]string{
	"ActionUser":         "action",
	"PartsInitLog":       ".parts-init.log",
	"NitrousboxrcFile":   ".nitrousboxrc",
	"NitrousboxrcSample": ".nitrousboxrc.sample",
	"NitrousboxrcLog":    ".nitrousboxrc.log",
}

const PartsInitConfTemplate = `
# parts-init
description "runs parts init as {{.ActionUser}} user"

task
start on (net-device-up
          and local-filesystems
          and runlevel [2345])

env HOME="/home/{{.ActionUser}}"
env PATH="/home/{{.ActionUser}}/.parts/autoparts/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

setuid {{.ActionUser}}
setgid {{.ActionUser}}

console log

chdir /home/{{.ActionUser}}

script
  parts update || true
  AUTOPARTS_NO_AUTOUPDATE=1 parts init -
  parts init --start
end script
`

// TODO: this Upstart conf does not enforce timeouts on .nitrousboxrc execution
// time
const RunNitrousboxrcConfTemplate = `
# run-nitrousboxrc
description "runs /home/{{.ActionUser}}/{{.NitrousboxrcFile}} as {{.ActionUser}} user"

task
start on stopped parts-init

env HOME="/home/{{.ActionUser}}"
env BOXRC="/home/{{.ActionUser}}/{{.NitrousboxrcFile}}"
env BOXRC_LOG="/home/{{.ActionUser}}/{{.NitrousboxrcLog}}"
export HOME
export BOXRC
export BOXRC_LOG

setuid {{.ActionUser}}
setgid {{.ActionUser}}

kill timeout 5 # note: doesn't actually time out the $BOXRC run
console log

chdir /home/{{.ActionUser}}

pre-start script
  [ ! -x $BOXRC ] && { stop; exit 0; }
  echo "$(date +'%F %T') : Starting $BOXRC" >> $BOXRC_LOG
end script

exec $BOXRC >> $BOXRC_LOG 2>&1

post-stop script
  [ ! -x $BOXRC ] && exit 0
  echo "$(date +'%F %T') : Finished $BOXRC" >> $BOXRC_LOG
end script
`

const NitrousboxrcSampleTemplate = `#!/bin/bash

# Note: You can also use #!/bin/sh or #!/bin/zsh as the first line.

# Use {{.NitrousboxrcFile}} to run commands (start daemons, etc) when your box is
# started/restarted.
# The results of running this script will be appended to
# /home/{{.ActionUser}}/{{.NitrousboxrcLog}} .

# To use this file, run the following commands in your terminal:
#   cp ~/{{.NitrousboxrcSample}} ~/{{.NitrousboxrcFile}}
#   chmod +x ~/{{.NitrousboxrcFile}}

# The line below is recommended so that any errors will stop the script.
set -e

# Add your commands here...
`

const SudoerTemplate = `
# enable sudo right for action user without password
action ALL=(ALL) NOPASSWD: ALL
`

const AliasSudoTemplate = `
alias sudo="echo -e '-----\nWARNING: sudo access is not available; running without sudo.\n-----'; "
`

var (
	NetworkConfigCompiled               *template.Template
	LxcTemplateCompiled                 *template.Template
	HostsTemplateCompiled               *template.Template
	SudoerTemplateCompiled              *template.Template
	AliasSudoTemplateCompiled           *template.Template
	PartsInitConfTemplateCompiled       *template.Template
	RunNitrousboxrcConfTemplateCompiled *template.Template
	NitrousboxrcSampleTemplateCompiled  *template.Template
)

var (
	ReadmeTemplateCompiled *template.Template
	TestTemplateCompiled   *template.Template
	RailsTemplateCompiled  *template.Template
	DjangoTemplateCompiled *template.Template
	NodejsTemplateCompiled *template.Template
	GoTemplateCompiled     *template.Template
	PhpTemplateCompiled    *template.Template
	RuntimeTemplates       ReadmeTemplates
)

type ReadmeTemplates map[string]*template.Template

func (r ReadmeTemplates) Get(name string) *template.Template {
	t, ok := r[name]
	if !ok {
		return ReadmeTemplateCompiled
	}
	return t
}

func init() {
	HostsTemplateCompiled = template.Must(template.New("hosts").Parse(HostsTemplate))
	NetworkConfigCompiled = template.Must(template.New("network").Parse(NetworkConfigTemplate))
	LxcTemplateCompiled = template.Must(template.New("lxc").Parse(LxcTemplate))
	SudoerTemplateCompiled = template.Must(template.New("sudoer").Parse(SudoerTemplate))
	AliasSudoTemplateCompiled = template.Must(template.New("alias_sudo").Parse(AliasSudoTemplate))
	PartsInitConfTemplateCompiled = template.Must(template.New("parts-init.conf").Parse(PartsInitConfTemplate))
	RunNitrousboxrcConfTemplateCompiled = template.Must(template.New("run-nitrousboxrc.conf").Parse(RunNitrousboxrcConfTemplate))
	NitrousboxrcSampleTemplateCompiled = template.Must(template.New("nitrousboxrc.sample").Parse(NitrousboxrcSampleTemplate))

	ReadmeTemplateCompiled = template.Must(template.New("readme_template").Parse(ReadmeTemplate))
	TestTemplateCompiled = template.Must(template.New("test_template").Parse(TestTemplate))
	RailsTemplateCompiled = template.Must(template.New("rails_template").Parse(RailsTemplate))
	DjangoTemplateCompiled = template.Must(template.New("django_template").Parse(DjangoTemplate))
	NodejsTemplateCompiled = template.Must(template.New("nodejs_template").Parse(NodejsTemplate))
	GoTemplateCompiled = template.Must(template.New("go_template").Parse(GoTemplate))
	PhpTemplateCompiled = template.Must(template.New("php_template").Parse(PhpTemplate))

	RuntimeTemplates = ReadmeTemplates{
		"test":   TestTemplateCompiled,
		"rails":  RailsTemplateCompiled,
		"django": DjangoTemplateCompiled,
		"nodejs": NodejsTemplateCompiled,
		"go":     GoTemplateCompiled,
		"php":    PhpTemplateCompiled,
	}
}
