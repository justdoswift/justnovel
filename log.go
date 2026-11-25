package main

import (
	"fmt"
	"time"
)

// LogKind is what a line of the execution log is about.
type LogKind int

const (
	LogInfo LogKind = iota
	LogChapter
	LogRetry
	LogFallback
	LogWarn
	LogError
	LogDone
)

// LogLine is one thing the run did, with the time it happened.
type LogLine struct {
	Seq    int
	At     time.Time
	Kind   LogKind
	Text   string
	Detail string
	// Took is how long the step took, when that is worth showing.
	Took time.Duration
}

// maxLogLines caps the log so a long book cannot grow it without bound. The
// oldest lines fall off the top; the run's tail is what matters.
const maxLogLines = 400

// shortDuration formats a duration the way the log's column does.
func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()+0.5))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()+0.5))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) - hours*60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
