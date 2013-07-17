package s3

import (
	"net/url"
	"path/filepath"
)

type Url struct {
	Region, Bucket, File string
}

func ParseUrl(s3Url string) (u Url, err error) {
	frag, err := url.Parse(s3Url)
	if err != nil {
		return
	}
	bucket := filepath.Dir(frag.Path)

	u.Region = frag.Host
	u.File = filepath.Base(frag.Path)
	u.Bucket = bucket[1:len(bucket)] // remove / from bucketname
	return
}
