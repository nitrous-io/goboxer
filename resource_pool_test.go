package goboxer

//import (
//    "github.com/garyburd/redigo/redis"
//    "testing"
//)

//type ResourcePoolSuite struct {
//    conn redis.Conn
//}

//func newResourcePoolSuite(t *testing.T) *ResourcePoolSuite {
//    conn, err := redis.Dial("tcp", ":6379")
//    if err != nil {
//        t.Fatal("could not connect to redis at port 6379")
//    }
//    conn.Send("Select", 13)
//    conn.Send("FLUSHDB")
//    conn.Flush()
//    return &ResourcePoolSuite{conn: conn}
//}

//func (s *ResourcePoolSuite) insertResource(ip, gateway, networkInterface, port string) {
//    r := &Resource{gateway: gateway, networkInterface: networkInterface, port: port}
//    rjson, _ := r.ToJSON()
//    s.conn.Send("RPUSH", "aio:available_resources", ip)
//    s.conn.Send("HMSET", "aio:nwr:"+ip, rjson)
//    s.conn.Flush()
//}

//func (s *ResourcePoolSuite) tearDown() {
//    s.conn.Send("FLUSHDB")
//    s.conn.Flush()
//    s.conn.Close()
//}

//func testAcquireAvailableResource(t *testing.T) {
//    suite := newResourcePoolSuite(t)
//    suite.insertResource("192.168.2.2", "192.168.2.1", "br-lxc-254", "10002")
//    rs := NewResourcePool(suite.conn)
//    r, err := rs.Acquire()
//    if err != nil {
//        t.Error("expect acquire to success")
//    }
//    if r.gateway != "192.168.2.1" {
//        t.Errorf("Expect gateway: %s, but got %s", "192.168.2.1", r.gateway)
//    }
//    if r.networkInterface != "br-lxc-254" {
//        t.Errorf("Expect network interface %s, but got %s", "br-lxc-254", r.networkInterface)
//    }
//    if r.port != "10002" {
//        t.Errorf("Expect port %s, but got %s", "10002", r.port)
//    }
//    suite.tearDown()
//}

//func testAcquireNonAvailableResource(t *testing.T) {
//    suite := newResourcePoolSuite(t)
//    rs := NewResourcePool(suite.conn)
//    r, err := rs.Acquire()
//    if r != nil || err == nil {
//        t.Error("should not acquire any resources")
//    }
//    suite.tearDown()
//}

//func TestReleaseValidResource(t *testing.T) {
//}
