package resource

import (
	"testing"
)

func TestToJSON(t *testing.T) {
	resource := &Resource{
		Ip:               "192.168.0.253",
		Gateway:          "192.168.0.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10085"}
	rjson, err := resource.ToJSON()
	if err != nil {
		t.Error("Failed to convert to json", err)
	}
	expected := "{\"ip\":\"192.168.0.253\",\"gateway\":\"192.168.0.1\",\"interface\":\"br-lxc-254\",\"port\":\"10085\"}"
	if rjson != expected {
		t.Error("Expected ", expected, "got ", rjson)
	}
}
