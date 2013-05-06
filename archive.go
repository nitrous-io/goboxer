package goboxer

import (
	"errors"
	"os/exec"
)

func Untar(archive, path string) error {
	cmd := exec.Command("tar", "-x", "-z", "-S", "-p", "-f", archive, "-C", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(err.Error() + ": " + string(output))
	}
	return nil
}
