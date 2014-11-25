package quota

import (
	"fmt"
	"github.com/nitrous-io/goboxer/utils"
	"os/exec"
	"strconv"
	"strings"
)

type PrjQuota struct{}

func (q PrjQuota) SetLimit(arg QuotaArg) error {
	quotaInfo, err := q.ViewLimit(arg)
	if err != nil {
		return err
	}
	newLimit := ConvertValueUnitToByte(ParseValueUnit(arg.Limit))
	if quotaInfo.Used >= newLimit {
		return fmt.Errorf("Current usage %d exceeded new limit %d",
			quotaInfo.Used, newLimit)
	}
	if err := utils.AppendlnUnlessContain("/etc/projid", arg.Name+":"+arg.Id); err != nil {
		return err
	}
	if err := utils.AppendlnUnlessContain("/etc/projects", arg.Id+":"+arg.Path); err != nil {
		return err
	}
	// assume that xfs_quota works here
	quotaValue := fmt.Sprintf("%d%s", int(ScaleUnit(newLimit, "M")), "M")
	xfsquota("project -s "+arg.Name, arg.Disk)
	xfsquota(fmt.Sprintf("limit -p bhard=%s %s", quotaValue, arg.Name),
		arg.Disk)
	return nil
}

func (q PrjQuota) RemoveLimit(arg QuotaArg) error {
	if err := utils.DeleteLineContains("/etc/projid", arg.Name+":"+arg.Id); err != nil {
		return err
	}
	if err := utils.DeleteLineContains("/etc/projects", arg.Path); err != nil {
		return err
	}
	xfsquota("limit -p bhard=0 "+arg.Id, arg.Disk)
	return nil
}

func (q PrjQuota) ViewLimit(arg QuotaArg) (QuotaInfo, error) {
	stdout, err := xfsquota("quota -pvN "+arg.Id, arg.Disk)
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
