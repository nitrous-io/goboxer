package quota

import (
	"os/exec"
)

func RunCommand(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}
