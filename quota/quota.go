package quota

type QuotaInfo struct {
	Used  int
	Limit int
}

// Id - id of the box
// Name - name of the box
// Path - path that we want to enforce quota ( for prjquota )
// Disk - disk that we want to enforce quota ( for prjquota and usrquota )
// Limit - quota limit that we want to enforce ( e.g 750M )
type QuotaArg struct {
	Id    string
	Name  string
	Path  string
	Disk  string
	Limit string
}

type QuotaInterface interface {
	SetLimit(arg QuotaArg) error
	RemoveLimit(arg QuotaArg) error
	ViewLimit(arg QuotaArg) (QuotaInfo, error)
}

// retrieve the proper quota provisioner depends on the provided method
//
// We specifically supports MountMethod: "failed" here so we dont have to
// inject special quota instance in test. This should never happens in
// production
type provisioner bool

var Provisioner provisioner = true

func (p provisioner) GetQuota(method string) QuotaInterface {
	if p {
		if method == "lvm" {
			return LvmQuota{}
		} else if method == "usrquota" {
			return UsrQuota{}
		}
		return PrjQuota{}
	} else {
		return FailQuota{}
	}
}
