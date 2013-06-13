package goboxer

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

func SetQuotaLimit(id, name, path, projectContainer, limit string) error {
	quotaInfo, err := ViewQuotaLimit(id, projectContainer)
	if err != nil {
		return err
	}
	newLimit := ConvertValueUnitToByte(ParseValueUnit(limit))
	if quotaInfo.Used >= newLimit {
		return fmt.Errorf("Current usage %d exceeded new limit %d",
			quotaInfo.Used, newLimit)
	}
	if err := Appendln("/etc/projid", name+":"+id); err != nil {
		return err
	}
	if err := Appendln("/etc/projects", id+":"+path); err != nil {
		return err
	}
	// assume that xfs_quota works here
	quotaValue := fmt.Sprintf("%d%s", int(ScaleUnit(newLimit, "M")), "M")
	xfsquota("project -s "+name, projectContainer)
	xfsquota(fmt.Sprintf("limit -p bhard=%s %s", quotaValue, name), projectContainer)
	return nil
}

func RemoveQuotaLimit(id, name, path, projectContainer string) error {
	if err := DeleteLineContains("/etc/projid", name+":"+id); err != nil {
		return err
	}
	if err := DeleteLineContains("/etc/projects", path); err != nil {
		return err
	}
	xfsquota("limit -p bhard=0 "+id, projectContainer)
	return nil
}

func ViewQuotaLimit(id, projectContainer string) (QuotaInfo, error) {
	stdout, err := xfsquota("quota -pvN "+id, projectContainer)
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
