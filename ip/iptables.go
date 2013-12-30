package ip

func Open(hostPort, containerIp, containerPort string) error {
	return forward("PREROUTING", hostPort, containerIp+":"+containerPort)
}

func OpenInVagrant(hostPort, containerIp, containerPort string) error {
	if err := Open(hostPort, containerIp, containerPort); err != nil {
		return err
	}
	return forward("OUTPUT", hostPort, containerIp+":"+containerPort)
}

func Close(hostPort, containerIp, containerPort string) error {
	return release("PREROUTING", hostPort, containerIp+":"+containerPort)
}

func CloseInVagrant(hostPort, containerIp, containerPort string) error {
	if err := Close(hostPort, containerIp, containerPort); err != nil {
		return err
	}
	return release("OUTPUT", hostPort, containerIp+":"+containerPort)
}
