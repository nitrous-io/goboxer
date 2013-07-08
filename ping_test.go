package goboxer

import (
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

type PingSuite struct{}

var _ = Suite(&PingSuite{})

func (s *PingSuite) TestPing(c *C) {
	mqConn, _ := redis.Dial("tcp", "127.0.0.1:65000")
	storeConn, _ := redis.Dial("tcp", "127.0.0.1:65000")
	ts := httptest.NewServer(http.HandlerFunc(pingHandler(mqConn,
		storeConn)))
	defer ts.Close()
	defer mqConn.Close()
	defer storeConn.Close()
	res, _ := http.Get(ts.URL)
	greeting, _ := ioutil.ReadAll(res.Body)
	res.Body.Close()
	c.Assert(res.StatusCode, Equals, 200)
	c.Assert(string(greeting), Equals, "ok")
}

type failConn struct{}

func (c failConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	return nil, redis.Error("failed")
}

func (c failConn) Close() error {
	return nil
}

func (s *PingSuite) TestPingFail(c *C) {
	mqConn := failConn{}
	storeConn := failConn{}
	ts := httptest.NewServer(http.HandlerFunc(pingHandler(mqConn,
		storeConn)))
	defer mqConn.Close()
	defer storeConn.Close()
	defer ts.Close()
	res, _ := http.Get(ts.URL)
	logger.Debugln(res.StatusCode, Equals, 500)
}
