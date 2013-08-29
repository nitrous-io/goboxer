package quotatest

import (
	"fmt"
	"github.com/action-io/goboxer/quota"
)

// test quota impl that will fail on everything
type FailQuota struct{}

func (q FailQuota) SetLimit(id, fileSystem, limit string) error {
	return fmt.Errorf("failed to set limit")
}
func (q FailQuota) RemoveLimit(id, fileSystem string) error {
	return fmt.Errorf("failed to remove limit")
}
func (q FailQuota) ViewLimit(id, fileSystem string) (quota.QuotaInfo, error) {
	return quota.QuotaInfo{}, fmt.Errorf("failed to view limit")
}
