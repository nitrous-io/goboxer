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

// a benchmark result will contains these keys together with their durations
// - aio:created
// - aio:started
// - aio:stopped
// - aio:destroyed
// - aio:invalid:params when things go wrong
func (b benchmarkResult) toString() string {
	var durations []interface{}
	format := ""
	for _, key := range []string{
		"aio:created",
		"aio:started",
		"aio:stopped",
		"aio:destroyed",
		"aio:invalid:params",
	} {
		val, ok := b[key]
		if ok {
			format += "%s\t"
			durations = append(durations, strconv.FormatFloat(val, 'f', 3, 64))
		}
	}
	return fmt.Sprintf(format, durations...)
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
			var err error
			start := time.Now()
			istr := strconv.Itoa(i)

			err = benchmarkCreate(boxer, istr)
			if err != nil {
				fmt.Println(err)
				return
			}

			err = benchmarkStart(boxer, istr)
			if err != nil {
				fmt.Println(err)
				return
			}

			err = benchmarkStop(boxer, istr)
			if err != nil {
				fmt.Println(err)
				return
			}

			err = benchmarkDestroy(boxer, istr)
			if err != nil {
				fmt.Println(err)
				return
			}

			collection.Add(istr, "total", time.Since(start).Seconds())
		}(i)
	}
	wg.Wait()
	fmt.Println("--> Summary")
	fmt.Println("Complete: ", time.Since(start).Seconds())
	printSummary()
}

// Create numbox and then benchmark Boxer.startAll methods
func StartBenchmarkAll(boxer *Boxer, numBox int) {
	// create numbox
	fmt.Println("--> Prepare boxes")
	for i := 2; i <= numBox+1; i++ {
		istr := strconv.Itoa(i)
		benchmarkCreate(boxer, istr)
	}

	// start all
	start := time.Now()
	boxer.Init()
	duration := time.Since(start).Seconds()

	// destroy numbox
	fmt.Println("--> CLEANUP")
	for i := 2; i <= numBox+1; i++ {
		istr := strconv.Itoa(i)
		benchmarkDestroy(boxer, istr)
	}

	fmt.Println("--> SUMMARY")
	fmt.Println("StartAll", strconv.FormatFloat(duration, 'f', 3, 64))
}

func benchmarkCreate(boxer *Boxer, id string) error {
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
	if message.Type == "aio:created" {
		collection.Add(id, message.Type, duration)
		return nil
	} else {
		return fmt.Errorf("Failed to create %s", id)
	}
}

func benchmarkStart(boxer *Boxer, id string) error {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:    boxer.Id,
		BoxId: id,
		Type:  "aio:start",
		Name:  "mclovins-container-" + id,
	})

	duration := time.Since(start).Seconds()
	if message.Type == "aio:started" {
		collection.Add(id, message.Type, duration)
		return nil
	} else {
		return fmt.Errorf("Failed to start %s", id)
	}
}

func benchmarkStop(boxer *Boxer, id string) error {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:    boxer.Id,
		BoxId: id,
		Type:  "aio:stop",
		Name:  "mclovins-container-" + id,
	})

	duration := time.Since(start).Seconds()
	if message.Type == "aio:stopped" {
		collection.Add(id, message.Type, duration)
		return nil
	} else {
		return fmt.Errorf("Failed to stop %s", id)
	}
}

func benchmarkDestroy(boxer *Boxer, id string) error {
	start := time.Now()
	message := boxer.Receive(Message{
		Id:    boxer.Id,
		BoxId: id,
		Type:  "aio:destroy",
		Name:  "mclovins-container-" + id,
	})
	duration := time.Since(start).Seconds()
	if message.Type == "aio:destroyed" {
		collection.Add(id, message.Type, duration)
		return nil
	} else {
		return fmt.Errorf("Failed to destroy %s", id)
	}
}

func printSummary() {
	fmt.Println("--> Details:")
	fmt.Printf("id:\taio:created\t\aio:started\t\aio:stopped\taio:destroyed\taio:invalid:params\n")
	for id, res := range collection.results {
		fmt.Printf("%v\t%v\n", id, res.toString())
	}
}
