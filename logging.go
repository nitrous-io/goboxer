// This contains some logging utilities
package goboxer

import (
	"log"
	"time"
)

type debugging bool

// By default, development mode and test mode can set this to true
// default to debug mode. This can be set in compile flag to false
// by `-ldflags "-X goboxer.debugging false"`
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

// provide a method to log duration of the function
// for now this is the only function signature we are going to accept
func (d debugging) LogHandler(name string, handler MessageHandler) MessageHandler {
	d.Debugf("%s: Started", name)
	return func(m Message, boxer *Boxer) Message {
		start := time.Now()
		msg := handler(m, boxer)
		d.Debugf("%s: %v", name, time.Since(start).Seconds())
		return msg
	}
}

func (d debugging) LogDuration(msg string) func(time.Time) {
	d.Printf("%s: Started", msg)
	return func(start time.Time) {
		d.Println(msg, time.Since(start).Seconds())
	}
}
