package quota

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type QuotaInfo struct {
	Used  int
	Limit int
}

type QuotaInterface interface {
	SetLimit(id, projectContainer, limit string) error
	RemoveLimit(id, projectContainer string) error
	ViewLimit(id, projectContainer string) (QuotaInfo, error)
}

type Quota struct{}

// this should be the same as the uid that we used as base to calculate user id
// a bit hacky now as we can pass the real id to methods instead of having to
// calculate it with this
const UidStart = 15000

func uid(id string) (uid int, err error) {
	intid, err := strconv.Atoi(id)
	if err != nil {
		return
	}
	uid = UidStart + intid
	return
}

func (q Quota) SetLimit(id, projectContainer, limit string) error {
	quotaInfo, err := q.ViewLimit(id, projectContainer)
	if err != nil {
		return err
	}
	newLimit := ConvertValueUnitToByte(ParseValueUnit(limit))
	if quotaInfo.Used >= newLimit {
		return fmt.Errorf("Current usage %d exceeded new limit %d",
			quotaInfo.Used, newLimit)
	}
	// assume that xfs_quota works here
	quotaValue := fmt.Sprintf("%d%s", int(ScaleUnit(newLimit, "M")), "M")
	user_id, err := uid(id)
	if err != nil {
		return err
	}
	xfsquota(fmt.Sprintf("limit bhard=%s %d", quotaValue, user_id),
		projectContainer)
	return nil
}

func (q Quota) RemoveLimit(id, projectContainer string) error {
	user_id, err := uid(id)
	if err != nil {
		return err
	}
	xfsquota(fmt.Sprintf("limit bhard=0 %d", user_id), projectContainer)
	return nil
}

func (q Quota) ViewLimit(id, projectContainer string) (QuotaInfo, error) {
	user_id, err := uid(id)
	if err != nil {
		return QuotaInfo{}, err
	}
	stdout, err := xfsquota(fmt.Sprintf("quota -vN %d", user_id),
		projectContainer)
	if err != nil {
		return QuotaInfo{}, err
	}
	fields := strings.Fields(string(stdout))
	if len(fields) == 0 {
		return QuotaInfo{}, nil
	}
	used, _ := strconv.Atoi(fields[1])
	limit, _ := strconv.Atoi(fields[3])
	return QuotaInfo{Used: used, Limit: limit}, nil
}

func xfsquota(arg, path string) ([]byte, error) {
	cmd := fmt.Sprintf("xfs_quota -x -c '%s' %s", arg, path)
	return exec.Command("bash", "-c", cmd).CombinedOutput()
}
