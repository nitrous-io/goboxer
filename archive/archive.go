package archive

import (
	"fmt"
	"io"
	"os/exec"
)

func Untar(archive, path string) error {
	cmd := exec.Command("tar", "-x", "-z", "-S", "-p", "-f", archive, "-C", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), string(output))
	}
	return nil
}

// tar the specified file path with gzip compression
func Tar(tarfilePath, pathToArchive string) error {
	cmd := exec.Command("tar", "-C", pathToArchive, "--numeric-owner", "-czpf", tarfilePath, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), string(output))
	}
	return nil
}

// Untar reads a stream of bytes from `archive`, parses it as a tar archive,
// and unpacks it into the directory at `path`.
// The archive should be gzip
func UntarStream(archive io.ReadCloser, path string) error {
	if archive == nil {
		return fmt.Errorf("UntarStream: empty archive")
	}
	cmd := exec.Command("tar", "--numeric-owner", "-xzSpf", "-", "-C", path)
	cmd.Stdin = archive
	// Hardcode locale environment for predictable outcome regardless of host configuration.
	// (see https://github.com/dotcloud/docker/issues/355)
	cmd.Env = []string{"LANG=en_US.utf-8", "LC_ALL=en_US.utf-8"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("UntarStream: err %v and output %v", err, output)
	}
	return archive.Close()
}
