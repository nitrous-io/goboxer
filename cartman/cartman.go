package cartman

import (
	"fmt"
	"net/http"
	"net/url"
)

type Cartman struct {
	Host string
}

func (c *Cartman) IsValid() bool {
	return len(c.Host) > 0
}

func (c *Cartman) InvalidateCache(containerName string) error {
	path := &url.URL{
		Scheme: "http",
		Host:   c.Host,
		Path:   "/invalidate_cache",
	}
	v := url.Values{}
	v.Set("id", containerName)
	path.RawQuery = v.Encode()
	res, err := http.Get(path.String())
	if err != nil || res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: Failed to invalidate cartman cached, status: %s",
			containerName, res.Status)
	}
	return nil
}
