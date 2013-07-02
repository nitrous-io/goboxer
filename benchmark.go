// Goboxer's benchmark mode. You find this here first
package goboxer

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"
)

type benchmarkResult map[string]float64

func (b *benchmarkResult) toString() string {
	s, _ := ToJSON(b)
	return s
}

type benchmarkCollection struct {
	sync.Mutex
	results map[string]benchmarkResult
}

func (b *benchmarkCollection) Add(id string, key string, val float64) {
	b.Lock()
	defer b.Unlock()
	res, ok := b.results[id]
	if ok {
		res[key] = val
	} else {
		r := benchmarkResult{}
		r[key] = val
		b.results[id] = r
	}
}

var collection *benchmarkCollection

func init() {
	collection = &benchmarkCollection{results: map[string]benchmarkResult{}}
}

// The benchmark will go:
// - create a number of box
// - start those boxes
// - destroy those boxes
// while log various statistics about these operations
//
// This assumes that we have valida redis data so we dont have to set them up
// again
func StartBenchmark(boxer *Boxer, numBox int) {
	log.Printf("Start benchmark mode with %d boxes ...\n", numBox)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 2; i <= numBox+1; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			istr := strconv.Itoa(i)

			benchmarkCreate(boxer, istr)
			benchmarkStart(boxer, istr)
			benchmarkStop(boxer, istr)
			benchmarkDestroy(boxer, istr)

			collection.Add(istr, "total", time.Since(start).Seconds())
		}(i)
	}
	wg.Wait()
	fmt.Println("--> Summary")
	fmt.Println("Complete: ", time.Since(start).Seconds())
	printSummary()
}

func benchmarkCreate(boxer *Boxer, id string) {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:         boxer.Id,
		BoxId:      id,
		Name:       "mclovins-container",
		Type:       "aio:create",
		Runtime:    "nodejs",
		Memory:     "256M",
		DiskSpace:  "1G",
		PublicKeys: []string{"ssh-key QWER", "ssh-key ASDF"},
	})

	duration := time.Since(start).Seconds()
	collection.Add(id, message.Type, duration)
}

func benchmarkStart(boxer *Boxer, id string) {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:    boxer.Id,
		BoxId: id,
		Type:  "aio:start",
		Name:  "mclovins-container-" + id,
	})

	duration := time.Since(start).Seconds()
	collection.Add(id, message.Type, duration)
}

func benchmarkStop(boxer *Boxer, id string) {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:    boxer.Id,
		BoxId: id,
		Type:  "aio:stop",
		Name:  "mclovins-container-" + id,
	})

	duration := time.Since(start).Seconds()
	collection.Add(id, message.Type, duration)
}

func benchmarkDestroy(boxer *Boxer, id string) {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:    boxer.Id,
		BoxId: id,
		Type:  "aio:destroy",
		Name:  "mclovins-container-" + id,
	})
	duration := time.Since(start).Seconds()
	collection.Add(id, message.Type, duration)
}

func printSummary() {
	fmt.Println("--> Details:")
	for id, res := range collection.results {
		fmt.Printf("%v		%v\n", id, res.toString())
	}
}
