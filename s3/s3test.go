package s3

import (
	"fmt"
)

// implement ClientInterface
type ClientTest struct {
	BucketName string
}

// do nothing, successfully
func (c *ClientTest) Upload(name, filePath string) (remotePath string, err error) {
	remotePath = fmt.Sprintf("s3://%s/%s", c.BucketName, name)
	return
}
