// interact with s3
package s3

import (
	"fmt"
	"io"
	"launchpad.net/goamz/aws"
	aws3 "launchpad.net/goamz/s3"
	"os"
)

type AuthInterface interface {
	NewClient(region, bucketName string) (ClientInterface, error)
}

type Auth aws.Auth

type ClientInterface interface {
	Upload(name, filePath string) (string, error)
	Download(name string) (io.ReadCloser, error)
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

// given a filename inside a bucket, return `io.ReadCloser` which we can use to
// read content of that file
func (c *Client) Download(filename string) (rc io.ReadCloser, err error) {
	s := aws3.New(c.Auth, c.Region)
	bucket := s.Bucket(c.BucketName)
	return bucket.GetReader(filename)
}

func (a Auth) NewClient(region, bucketName string) (client ClientInterface, err error) {
	awsRegion, ok := aws.Regions[region]
	if !ok {
		err = fmt.Errorf("Invalid region: %s", region)
		return
	}
	client = &Client{
		Auth:       aws.Auth(a),
		Region:     awsRegion,
		BucketName: bucketName,
	}
	return
}
