// this prepare the network for boxhost so we dont have to use shell scripts
// for those sort of things
package main

import (
	"flag"
	"fmt"
	"github.com/action-io/goboxer/ip"
	"strconv"
)

const START_PORT = 10000

func main() {
	// initialize number of boxes
	flStartOctet := flag.Int("s", 204, "start octet of the network interfaces")
	flEndOctet := flag.Int("e", 254, "end octet of the network interfaces")
	flIsVagrant := flag.Bool("v", false, "if the box host is a vagrant box")

	// setup the network masquerade rules
	for i := *flStartOctet; i <= *flEndOctet; i++ {
		source := fmt.Sprintf("192.168.%d.0/24", i)
		ip.CreateNetwork(source, *flIsVagrant)
	}

	// setup port forwarding rules
	portId := 0
	for i := *flEndOctet; i >= *flStartOctet; i-- {
		for j := 254; j >= 2; j-- {
			ipAddr := fmt.Sprintf("192.168.%d.%d", i, j)
			port := START_PORT + portId
			portId++
			ip.Open(strconv.Itoa(port), ipAddr, "22")
		}
	}
}

// setup the network masquerade rules
func setNetwork(start, end int, isVagrant bool) {
	for i := start; i <= end; i++ {
		source := fmt.Sprintf("192.168.%d.0/24", i)
		ip.CreateNetwork(source, isVagrant)
	}
}

// setup port forwarding rules
func setPortForwarding(start, end int, isVagrant bool) {
	portId := 0
	for i := end; i >= start; i-- {
		for j := 254; j >= 2; j-- {
			ipAddr := fmt.Sprintf("192.168.%d.%d", i, j)
			port := START_PORT + portId
			portId++
			if isVagrant {
				ip.OpenInVagrant(strconv.Itoa(port), ipAddr, "22")
			} else {
				ip.Open(strconv.Itoa(port), ipAddr, "22")
			}
		}
	}
}
