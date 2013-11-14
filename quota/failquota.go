package quota

import (
	"fmt"
)

// test quota impl that will fail on everything
type FailQuota struct{}

func (q FailQuota) SetLimit(arg QuotaArg) error {
	return fmt.Errorf("failed to set limit")
}
func (q FailQuota) RemoveLimit(arg QuotaArg) error {
	return fmt.Errorf("failed to remove limit")
}
func (q FailQuota) ViewLimit(arg QuotaArg) (QuotaInfo, error) {
	return QuotaInfo{}, fmt.Errorf("failed to view limit")
}
