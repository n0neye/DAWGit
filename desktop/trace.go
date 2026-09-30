package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// With DAWGIT_TRACE set, slow calls log how long each step took.
var tracing = os.Getenv("DAWGIT_TRACE") != ""

type stopwatch struct {
	name  string
	start time.Time
	last  time.Time
	laps  []string
}

func startWatch(name string) *stopwatch {
	now := time.Now()
	return &stopwatch{name: name, start: now, last: now}
}

// lap records the time since the previous lap under a step name.
func (s *stopwatch) lap(step string) {
	if !tracing {
		return
	}
	now := time.Now()
	s.laps = append(s.laps, fmt.Sprintf("%s %s", step, now.Sub(s.last).Round(time.Millisecond)))
	s.last = now
}

func (s *stopwatch) done() {
	if tracing {
		log.Printf("trace %s: %s total — %s", s.name, time.Since(s.start).Round(time.Millisecond), strings.Join(s.laps, ", "))
	}
}
