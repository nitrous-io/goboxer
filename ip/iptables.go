package ip

func Open(hostPort, containerIp, containerPort string) error {
	rule := natRule{"PREROUTING", hostPort, containerIp + ":" + containerPort}
	return rule.create()
}

func OpenInVagrant(hostPort, containerIp, containerPort string) error {
	if err := Open(hostPort, containerIp, containerPort); err != nil {
		return err
	}
	rule := natRule{"OUTPUT", hostPort, containerIp + ":" + containerPort}
	return rule.create()
}

func Close(hostPort, containerIp, containerPort string) error {
	rule := natRule{"PREROUTING", hostPort, containerIp + ":" + containerPort}
	return rule.destroy()
}

func CloseInVagrant(hostPort, containerIp, containerPort string) error {
	if err := Close(hostPort, containerIp, containerPort); err != nil {
		return err
	}
	rule := natRule{"OUTPUT", hostPort, containerIp + ":" + containerPort}
	return rule.destroy()
}
