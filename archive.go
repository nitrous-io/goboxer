package goboxer

import (
	"errors"
	"os/exec"
	"path/filepath"
)

func Untar(archive, path string) error {
	cmd := exec.Command("tar", "-x", "-z", "-S", "-p", "-f", archive, "-C", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(err.Error() + ": " + string(output))
	}
	return nil
}

// tar the specified file path with gzip compression
func Tar(name, path string) (tarFile string, err error) {
	cmd := exec.Command("tar", "-C", path, "--numeric-owner", "-czpf", name, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = errors.New(err.Error() + ": " + string(output))
		return
	}
	return filepath.Abs(name)
}
