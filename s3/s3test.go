package s3

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
)

// a tar file that has empty test.txt as content
var TEST = []byte{
	31, 139, 8, 0, 62, 192, 29, 81, 0, 3, 237, 209, 61, 10, 194, 64, 16, 6,
	208, 173, 61, 69, 78, 32, 187, 186, 73, 206, 19, 196, 34, 141, 130, 25, 193,
	227, 155, 128, 130, 141, 136, 69, 16, 225, 189, 230, 99, 126, 138, 129, 137,
	227, 20, 219, 184, 69, 90, 81, 158, 117, 93, 93, 178, 244, 109, 121, 205, 135,
	54, 149, 93, 201, 253, 190, 230, 220, 207, 253, 165, 168, 169, 201, 107, 30,
	245, 116, 157, 98, 184, 52, 77, 26, 14, 49, 158, 79, 239, 247, 62, 205, 255,
	84, 204, 255, 223, 252, 250, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 190, 118, 7, 60,
	232, 40, 50, 0, 40, 0, 0,
}

// implement AuthInterface
type AuthTest struct{}

func (a AuthTest) NewClient(region, bucketName string) (client ClientInterface, err error) {
	return &ClientTest{BucketName: bucketName}, nil
}

// implement ClientInterface
type ClientTest struct {
	BucketName string
}

// do nothing, successfully
func (c *ClientTest) Upload(name, filePath string) (remotePath string, err error) {
	remotePath = fmt.Sprintf("s3://%s/%s", c.BucketName, name)
	return
}

// given a filename inside a bucket, return `io.ReadCloser` which we can use to
// read content of that file
// Here we just stub the content with test-snapshot.tar.gz
func (c *ClientTest) Download(filename string) (rc io.ReadCloser, err error) {
	return ioutil.NopCloser(bytes.NewReader(TEST)), err
}
