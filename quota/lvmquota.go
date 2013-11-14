package quota

import (
	"bufio"
	"fmt"
	"github.com/action-io/goboxer/mount"
	"github.com/action-io/goboxer/utils"
	"os/exec"
	"strconv"
	"strings"
)

const FS_TYPE = "ext4"

type LvmQuota struct{}

func (q LvmQuota) SetLimit(arg QuotaArg) error {
	lv := "/dev/aio_boxhost/" + arg.Name
	return utils.RunCommand("lvresize", lv, "--size", arg.Limit, "--resizefs")
}

// just assume success here as we want to remove entire lvm volume
func (q LvmQuota) RemoveLimit(arg QuotaArg) error {
	return nil
}

func (q LvmQuota) ViewLimit(arg QuotaArg) (QuotaInfo, error) {
	mounted, err := mount.Mounted(arg.Path)
	if err != nil {
		return QuotaInfo{}, err
	}
	if !mounted {
		return QuotaInfo{}, fmt.Errorf("%s is not mounted", arg.Path)
	}
	cmd := exec.Command("df", arg.Path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return QuotaInfo{}, err
	}
	if err := cmd.Start(); err != nil {
		return QuotaInfo{}, err
	}
	scanner := bufio.NewScanner(stdout)
	quota := QuotaInfo{}
	for scanner.Scan() {
		line := scanner.Text()
		// found the quota for user
		fields := strings.Fields(line)
		if fields[5] == arg.Path {
			quota.Used, _ = strconv.Atoi(fields[2])
			quota.Limit, _ = strconv.Atoi(fields[1])
			break
		}
	}
	if err := cmd.Wait(); err != nil {
		return QuotaInfo{}, err
	}
	return quota, nil
}
