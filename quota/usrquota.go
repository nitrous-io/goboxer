package quota

/*
#include <sys/quota.h>
#include <stdlib.h>

static int get_quota(const char *special, int id, caddr_t addr) {
  return quotactl(QCMD(Q_GETQUOTA, USRQUOTA), special, id, addr);
}
*/
import "C"

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"
)

// this should be the same as the uid that we used as base to calculate user id
// a bit hacky now as we can pass the real id to methods instead of having to
// calculate it with this
const UidStart = 15000

type UsrQuota struct{}

func uid(id string) (uid int, err error) {
	intid, err := strconv.Atoi(id)
	if err != nil {
		return
	}
	uid = UidStart + intid
	return
}

func (q UsrQuota) SetLimit(arg QuotaArg) error {
	quotaInfo, err := q.ViewLimit(arg)
	if err != nil {
		return err
	}
	newLimit := ConvertValueUnitToByte(ParseValueUnit(arg.Limit))
	if quotaInfo.Used >= newLimit {
		return fmt.Errorf("Current usage %d exceeded new limit %d",
			quotaInfo.Used, newLimit)
	}
	setQuota(arg.Id, newLimit, arg.Disk)
	return nil
}

func (q UsrQuota) RemoveLimit(arg QuotaArg) error {
	setQuota(arg.Id, 0, arg.Disk)
	return nil
}

// Return QuotaInfo which describes current used space as well as limit of the
// current user. The number is measured in 1024 bytes (i.e. 1KB) by default.
func (q UsrQuota) ViewLimit(arg QuotaArg) (QuotaInfo, error) {
	quota := QuotaInfo{}
	userId, err := uid(arg.Id)
	if err != nil {
		return quota, err
	}
	var info C.struct_dqblk
	device, err := getDevice(arg.Disk)
	if err != nil {
		return quota, err
	}
	dev := C.CString(device)
	defer C.free(unsafe.Pointer(dev))
	if C.get_quota(dev, C.int(userId),
		(C.caddr_t)(unsafe.Pointer(&info))) == 1 {
		return quota, fmt.Errorf("Failed to get quota: %v", arg)
	}
	quota.Used = int(info.dqb_curspace)
	quota.Limit = int(info.dqb_bhardlimit)
	return quota, nil
}

func setQuota(id string, newLimit int, disk string) error {
	userId, err := uid(id)
	if err != nil {
		return err
	}
	uidStr := strconv.Itoa(userId)
	newLimitStr := strconv.Itoa(newLimit)
	cmd := exec.Command("setquota", "-u", uidStr, newLimitStr, newLimitStr,
		"0", "0", disk)
	return cmd.Run()
}

// given a device or mountpoint, get the underlying device name
func getDevice(deviceOrDir string) (string, error) {
	fstab, err := os.Open("/etc/mtab")
	if err != nil {
		return "", err
	}
	defer fstab.Close()
	scanner := bufio.NewScanner(fstab)
	device := ""
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == deviceOrDir || fields[1] ==
			deviceOrDir {
			device = fields[0]
			break
		}
	}
	return device, nil
}
