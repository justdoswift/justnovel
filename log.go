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

// logf appends a line to the run's log. Call it on the UI thread, or through
// win.Update from the crawler's goroutine.
func (a *app) logf(kind LogKind, text, detail string) {
	a.logSeq++
	a.log = append(a.log, LogLine{Seq: a.logSeq, At: time.Now(), Kind: kind, Text: text, Detail: detail})
	if len(a.log) > maxLogLines {
		a.log = a.log[len(a.log)-maxLogLines:]
	}
	// Follow the tail, the way a build log does.
	a.logState.FollowEnd = true
}

// logChapter records a chapter the crawler took, with how long it took and
// how it was fetched.
func (a *app) logChapter(order int, title string, transport Transport, took time.Duration) {
	detail := ""
	if transport == TransportBrowser {
		detail = "浏览器回退"
	}
	a.logSeq++
	a.log = append(a.log, LogLine{
		Seq: a.logSeq, At: time.Now(), Kind: LogChapter,
		Text: fmt.Sprintf("%03d  %s", order, title), Detail: detail, Took: took,
	})
	if len(a.log) > maxLogLines {
		a.log = a.log[len(a.log)-maxLogLines:]
	}
	a.logState.FollowEnd = true
}

// logReset clears the log for a new run.
func (a *app) logReset() {
	a.log = nil
	a.logSeq = 0
}
