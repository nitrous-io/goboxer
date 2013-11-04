package quota

import (
	"bufio"
	"fmt"
	"github.com/action-io/goboxer/mount"
	"os/exec"
	"strconv"
	"strings"
)

const FS_TYPE = "ext4"

type LvmQuota struct{}

func (q LvmQuota) SetLimit(arg QuotaArg) error {
	quotaInfo, err := q.ViewLimit(arg)
	if err != nil {
		return err
	}
	newLimit := ConvertValueUnitToByte(ParseValueUnit(arg.Limit))
	if quotaInfo.Used >= newLimit {
		return fmt.Errorf("Current usage %d exceeded new limit %d",
			quotaInfo.Used, newLimit)
	}
	if err := mount.Unmount(arg.Path); err != nil {
		return err
	}
	lv := "/dev/aio_boxhost/" + arg.Name
	exec.Command("lvresize", lv, "--size", arg.Limit, "--resizefs").Run()
	if err := mount.Mount(lv, arg.Path, FS_TYPE); err != nil {
		return err
	}
	return nil
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
