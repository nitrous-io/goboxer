// Goboxer's benchmark mode. You find this here first
package goboxer

import (
	"log"
)

// The benchmark will go:
// - create a number of box
// - start those boxes
// - destroy those boxes
// while log various statistics about these operations
func StartBenchmark(boxer *Boxer, numBox int) {
	log.Printf("Start benchmark mode with %d boxes ...\n", numBox)
}
