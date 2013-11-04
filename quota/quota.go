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
