package quota

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

func (q UsrQuota) ViewLimit(arg QuotaArg) (QuotaInfo, error) {
	userId, err := uid(arg.Id)
	if err != nil {
		return QuotaInfo{}, err
	}
	cmd := exec.Command("repquota", "-n", arg.Disk)
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
		if len(fields) > 1 && fields[0] == fmt.Sprintf("#%d", userId) {
			quota.Used, _ = strconv.Atoi(fields[2])
			quota.Limit, _ = strconv.Atoi(fields[3])
			break
		}
	}
	if err := cmd.Wait(); err != nil {
		return QuotaInfo{}, err
	}
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
