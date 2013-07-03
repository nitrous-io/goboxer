// This contains some logging utilities
package goboxer

import (
	"log"
)

type debugging bool

// default to debug mode. This can be set in compile flag to false
var logger debugging = true

func (d debugging) Printf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (d debugging) Println(args ...interface{}) {
	log.Println(args...)
}

func (d debugging) Debugf(format string, args ...interface{}) {
	if d {
		log.Printf(format, args...)
	}
}

func (d debugging) Debugln(args ...interface{}) {
	if d {
		log.Println(args...)
	}
}
