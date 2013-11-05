package quotatest

import (
	"fmt"
	"github.com/action-io/goboxer/quota"
)

// test quota impl that will fail on everything
type FailQuota struct{}

func (q FailQuota) SetLimit(arg quota.QuotaArg) error {
	return fmt.Errorf("failed to set limit")
}
func (q FailQuota) RemoveLimit(arg quota.QuotaArg) error {
	return fmt.Errorf("failed to remove limit")
}
func (q FailQuota) ViewLimit(arg quota.QuotaArg) (quota.QuotaInfo, error) {
	return quota.QuotaInfo{}, fmt.Errorf("failed to view limit")
}
