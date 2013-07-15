// interact with s3
package s3

import (
	"fmt"
	"launchpad.net/goamz/aws"
	aws3 "launchpad.net/goamz/s3"
	"os"
)

type Client struct {
	Auth   aws.Auth
	Region aws.Region
}

func (c *Client) Upload(name, filePath, bucketName string) (err error) {
	var file *os.File
	var info os.FileInfo
	if file, err = os.Open(filePath); err != nil {
		return
	}
	if info, err = file.Stat(); err != nil {
		return
	}

	s := aws3.New(c.Auth, c.Region)
	bucket := s.Bucket(bucketName)
	return bucket.PutReader(name, file, info.Size(), "binary/octet-stream",
		aws3.BucketOwnerFull)
}

func NewClient(accessKey, secretKey, region string) (client *Client, err error) {
	if accessKey == "" || secretKey == "" || region == "" {
		err = fmt.Errorf("Empty params detected %s, %s, %s", accessKey,
			secretKey, region)
		return
	}
	awsRegion, ok := aws.Regions[region]
	if !ok {
		err = fmt.Errorf("Invalid region: %s", region)
		return
	}
	client.Auth = aws.Auth{accessKey, secretKey}
	client.Region = awsRegion
	return
}
