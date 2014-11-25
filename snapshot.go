package goboxer

import (
	"github.com/nitrous-io/goboxer/quota"
)

type Snapshot struct {
	Id   int
	Url  string
	Size quota.ValueUnit // Uncompressed snapshot size in MB.
}

func (s *Snapshot) SizeInMB() int {
	return quota.ScaleUnitCeil(int(s.Size.Value), "M")
}
