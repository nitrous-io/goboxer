package cartman

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/motain/gocheck"
)

type CartmanSuite struct {
}

var _ = Suite(&CartmanSuite{})

func (s *CartmanSuite) TestInvalidateCache(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Hello, boxer")
		}))
	defer ts.Close()
	cartmanURL, _ := url.Parse(ts.URL)
	cartman := &Cartman{Host: cartmanURL.Host}

	err := cartman.InvalidateCache("some-container")
	c.Assert(err, IsNil)
}

func (s *CartmanSuite) TestInvalidateCacheWithoutServer(c *C) {
	cartman := &Cartman{Host: "http://0.0.0.1"}
	err := cartman.InvalidateCache("some-container")
	c.Assert(err, NotNil)
}
