// interact with s3
package s3

import (
	"fmt"
	"launchpad.net/goamz/aws"
	aws3 "launchpad.net/goamz/s3"
	"os"
)

type ClientInterface interface {
	Upload(name, filePath string) (string, error)
}

type Client struct {
	Auth       aws.Auth
	Region     aws.Region
	BucketName string
}

// TODO: might make the upload async by using a channel and goroutine here
func (c *Client) Upload(name, filePath string) (remotePath string, err error) {
	var file *os.File
	var info os.FileInfo
	if file, err = os.Open(filePath); err != nil {
		return
	}
	if info, err = file.Stat(); err != nil {
		return
	}

	s := aws3.New(c.Auth, c.Region)
	bucket := s.Bucket(c.BucketName)
	err = bucket.PutReader(name, file, info.Size(), "binary/octet-stream",
		aws3.BucketOwnerFull)
	if err != nil {
		return
	}
	remotePath = fmt.Sprintf("s3://%s/%s", c.BucketName, name)
	return
}

func NewClient(accessKey, secretKey, region, bucketName string) (client *Client, err error) {
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
	client = &Client{
		Auth:       aws.Auth{AccessKey: accessKey, SecretKey: secretKey},
		Region:     awsRegion,
		BucketName: bucketName,
	}
	return
}
