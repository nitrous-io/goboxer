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

type QuotaInfo struct {
	Used  int
	Limit int
}

type QuotaInterface interface {
	SetLimit(id, fileSystem, limit string) error
	RemoveLimit(id, fileSystem string) error
	ViewLimit(id, fileSystem string) (QuotaInfo, error)
}

type Quota struct{}

func uid(id string) (uid int, err error) {
	intid, err := strconv.Atoi(id)
	if err != nil {
		return
	}
	uid = UidStart + intid
	return
}

func (q Quota) SetLimit(id, fileSystem, limit string) error {
	quotaInfo, err := q.ViewLimit(id, fileSystem)
	if err != nil {
		return err
	}
	newLimit := ConvertValueUnitToByte(ParseValueUnit(limit))
	if quotaInfo.Used >= newLimit {
		return fmt.Errorf("Current usage %d exceeded new limit %d",
			quotaInfo.Used, newLimit)
	}
	userId, err := uid(id)
	if err != nil {
		return err
	}
	// ignore errors here
	err = setQuota(newLimit, newLimit, userId, fileSystem)
	if err != nil {
		fmt.Println("SetLimitError", err)
	}
	return nil
}

func (q Quota) RemoveLimit(id, fileSystem string) error {
	userId, err := uid(id)
	if err != nil {
		return err
	}
	err = setQuota(0, 0, userId, fileSystem)
	if err != nil {
		fmt.Println("RemoveLimitError", err)
	}
	return nil
}

func (q Quota) ViewLimit(id, fileSystem string) (info QuotaInfo, err error) {
	var userId int
	var fields []string
	if userId, err = uid(id); err != nil {
		return
	}
	if fields, err = viewQuota(userId, fileSystem); err != nil {
		return
	}
	if len(fields) == 0 {
		return
	}
	usedStr := strings.Replace(fields[2], "*", "", -1)
	limitStr := strings.Replace(fields[4], "*", "", -1)
	used, _ := strconv.Atoi(usedStr)
	limit, _ := strconv.Atoi(limitStr)
	return QuotaInfo{Used: used, Limit: limit}, nil
}

func setQuota(softLimit, hardLimit int, user int, fileSystem string) error {
	cmd := exec.Command("setquota", "-u", strconv.Itoa(user),
		strconv.Itoa(softLimit), strconv.Itoa(hardLimit), "0", "0", fileSystem)
	return cmd.Run()
}

func viewQuota(user int, fileSystem string) (fields []string, err error) {
	cmd := exec.Command("quota", "-wu", strconv.Itoa(user), "--show-mntpoint")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(stdout)
	cmd.Start()
	for scanner.Scan() {
		info := scanner.Text()
		fields = strings.Fields(info)
		if fields[1] == fileSystem {
			break
		}
	}
	if err = scanner.Err(); err != nil {
		return
	}
	cmd.Wait()
	return
}

func toString(input []byte, err error) (string, error) {
	return string(input), err
}
