package goboxer

import (
	"text/template"
)

const NetworkConfigTemplate = `
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address {{.IpAddress}}
    netmask 255.255.255.0
    gateway {{.NetworkInterface}}
`

var NetworkConfigCompiled *template.Template

func init() {
	var err error
	NetworkConfigCompiled, err = template.New("network").Parse(NetworkConfigTemplate)
	if err != nil {
		panic(err)
	}
}
