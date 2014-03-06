package s3

import (
	. "github.com/motain/gocheck"
)

type URLSuite struct{}

var _ = Suite(&URLSuite{})

func (s *URLSuite) TestParseURL(c *C) {
	url, err := ParseUrl("s3://ap-southeast-1/bucket/file.tar.gz")
	c.Assert(err, IsNil)
	c.Assert(url.Region, Equals, "ap-southeast-1")
	c.Assert(url.Bucket, Equals, "bucket")
	c.Assert(url.File, Equals, "file.tar.gz")
}
