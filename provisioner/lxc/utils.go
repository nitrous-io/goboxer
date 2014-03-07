package lxc

import (
	"strconv"
	"time"
)

func generateId() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
