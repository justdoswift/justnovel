package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

// Session owns the work: the site, the book, the run, and the phase they are
// in. It is the only place that decides what the app is doing.
//
// Phase is the single source of truth. "Is it busy", "can it pause", "does the
// browser have to be in front" are all derived from it. The old app kept a
// Phase plus five independent booleans that each meant something similar, and
// the catalog load deadlocked because two of them disagreed about whether the
// app was busy.
//
// Concurrency: mu guards every field below. The public methods lock; the
// *Locked helpers assume the lock is already held, so a method can call
// another without deadlocking. Goroutines reach the state through update,
// which is the one funnel that takes the lock and then redraws.
type Session struct {
	mu sync.Mutex

	win     *mygo.Window
	panel   Panel
	history *History

	source Source
	phase  Phase

	// message and detail explain the phase to the user.
	message string
	detail  string

	// input is what the user typed: a title to search, or an address to open.
	// The text field writes it directly on the UI thread, which is why it is
	// exposed as a pointer rather than through a setter.
	input string

	hits    []SearchHit
	catalog *BookCatalog

	chapters []Chapter
	next     int
	recordID string

	// navSeq numbers the page load the app is waiting for. A load event that
	// belongs to an older navigation is ignored, so a slow page from a
	// previous attempt cannot read a catalog the user no longer wants.
	navSeq int

	cancel context.CancelFunc
	// stopping tells the download goroutine that the run was stopped on
	// purpose, rather than paused. It is atomic because the goroutine reads it
	// while the UI thread writes it, without taking the lock.
	stopping atomic.Bool

	// progress is the run's snapshot. It is written with the phase, never on
	// its own, so a stale snapshot cannot outlive the run it describes.
	progress *Progress

	// log is the run's execution log, newest last: what the crawler did, in
	// the order it did it.
	log    []LogLine
	logSeq int

	// startedAt and startCount time the run, so the sheet can estimate what
	// is left from the pace it has actually kept.
	startedAt  time.Time
	startCount int

	lastSaved string
	lastErr   error

}

// NewSession starts a session on a site, with nothing open.
func NewSession(win *mygo.Window, panel Panel, history *History, source Source) *Session {
	return &Session{win: win, panel: panel, history: history, source: source, phase: PhaseIdle}
}

// Source is the site the session reads.
func (s *Session) Source() Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.source
}

// Phase is what the session is doing right now.
func (s *Session) Phase() Phase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

// Busy reports whether the session blocks a new piece of work. It asks the
// phase, so there is exactly one answer.
func (s *Session) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase.Busy()
}

// Message is the phase's headline detail, when it has one.
func (s *Session) Message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.message
}

// Detail is the longer explanation under the headline.
func (s *Session) Detail() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detail
}

// Input is the text field's binding: a title, or an address.
func (s *Session) Input() *string { return &s.input }

// Hits are the books the last search found.
func (s *Session) Hits() []SearchHit {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

// Catalog is the book the session read, if any.
func (s *Session) Catalog() *BookCatalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.catalog
}

// Chapters are the chapters downloaded so far.
func (s *Session) Chapters() []Chapter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chapters
}

// Next is the index of the chapter the run is on.
func (s *Session) Next() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

// LastSaved is the path of the last EPUB written.
func (s *Session) LastSaved() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSaved
}

// LastErr is the error that put the session in its failed phase.
func (s *Session) LastErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// Progress is the run's snapshot, or nil before a run starts.
func (s *Session) Progress() *Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

// Log is the run's execution log, oldest first.
func (s *Session) Log() []LogLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log
}

// Panel is the site's browser.
func (s *Session) Panel() Panel { return s.panel }

// WantsPanel reports whether the site's page has to be in front of the user:
// a search that has to run in the browser, or a site that asked for a human.
//
// It is derived from the phase, not set by a callback, so the window can ask
// it on every frame and get the same answer.
func (s *Session) WantsPanel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.phase {
	case PhaseSearching:
		return true
	case PhaseFailed:
		return s.lastErr != nil && needsManualVerification(s.lastErr)
	}
	return false
}

// CanPause reports whether the running download can be paused.
func (s *Session) CanPause() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase == PhaseDownloading
}

// CanResume reports whether a paused or failed download can continue.
func (s *Session) CanResume() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canResumeLocked()
}

func (s *Session) canResumeLocked() bool {
	if s.catalog == nil || s.cancel != nil {
		return false
	}
	if s.phase != PhasePaused && s.phase != PhaseFailed {
		return false
	}
	return s.next < len(s.catalog.Chapters)
}

// CanSkip reports whether the failed chapter can be dropped and the run
// continued.
func (s *Session) CanSkip() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canSkipLocked()
}

func (s *Session) canSkipLocked() bool {
	return s.phase == PhaseFailed && s.catalog != nil && s.next < len(s.catalog.Chapters)
}

// CanStart reports whether the catalog is ready and the run has not begun.
func (s *Session) CanStart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canStartLocked()
}

func (s *Session) canStartLocked() bool {
	return s.catalog != nil && s.phase == PhaseCatalogReady
}

// CanExport reports whether what is downloaded can be written as an EPUB.
func (s *Session) CanExport() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canExportLocked()
}

func (s *Session) canExportLocked() bool {
	return len(s.chapters) > 0 && !s.phase.Busy()
}

// HasResults reports whether a search found books to show.
func (s *Session) HasResults() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase == PhaseIdle && len(s.hits) > 0
}

// setPhaseLocked moves the session to a phase and records what it means. Every
// transition goes through here, so no field can disagree with the phase.
func (s *Session) setPhaseLocked(phase Phase, message, detail string) {
	s.phase = phase
	s.message = message
	s.detail = detail
	if !phase.Busy() {
		s.cancel = nil
	}
}

// failLocked records an error and the phase it leaves the session in.
func (s *Session) failLocked(err error) {
	s.lastErr = err
	s.setPhaseLocked(PhaseFailed, err.Error(), "")
}

// resetLocked forgets the book and its chapters, and stops whatever was
// running.
func (s *Session) resetLocked() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	s.stopping.Store(false)
	s.catalog = nil
	s.chapters = nil
	s.next = 0
	s.recordID = ""
	s.lastSaved = ""
	s.lastErr = nil
	s.progress = nil
	s.startedAt = time.Time{}
	s.startCount = 0
	s.setPhaseLocked(PhaseIdle, "", "")
}

// adopt switches the site, keeping the address when it already belongs to the
// new one.
func (s *Session) adopt(source Source) {
	s.mu.Lock()
	s.adoptLocked(source)
	s.mu.Unlock()
}

func (s *Session) adoptLocked(source Source) {
	if source.Kind == s.source.Kind {
		return
	}
	s.source = source
	if s.source.CatalogURL(s.input) == nil {
		s.input = ""
	}
	s.resetLocked()
	if s.panel != nil {
		s.panel.Configure(source)
	}
}

// statusDetail is the explanation under the status headline.
func (s *Session) statusDetail() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusDetailLocked()
}

func (s *Session) statusDetailLocked() string {
	switch s.phase {
	case PhaseIdle:
		return s.source.IdleDetail
	case PhaseSearching:
		return fmt.Sprintf("正在 %s 搜索「%s」。站点要验证时，在浏览器面板里完成。", s.source.Name, trimParagraph(s.input))
	case PhaseParsing:
		return "正在读取目录和章节信息。"
	case PhaseCatalogReady, PhasePaused, PhaseFinished, PhaseStopped, PhaseFailed:
		if s.detail != "" {
			return s.detail
		}
		return s.message
	case PhaseDownloading:
		if s.progress == nil {
			return "正在下载章节。"
		}
		p := s.progress
		text := fmt.Sprintf("已完成 %d / %d 章。\n正在处理：第 %d 章《%s》\n当前方式：%s",
			p.Completed, p.Total, p.CurrentIndex, p.CurrentTitle, p.Transport.Title())
		if p.HasLast {
			text += fmt.Sprintf("\n最近完成：第 %d 章《%s》", p.LastIndex, p.LastTitle)
		}
		return text + "\n" + p.Transport.Detail()
	case PhaseBuilding:
		return "正在写入目录、章节 XHTML 和 OPF 文件。"
	}
	return ""
}

// reset is the test-facing reset.
func (s *Session) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetLocked()
}

// setPhase is the test-facing phase setter: it takes the lock and records the
// phase the way every transition does.
func (s *Session) setPhase(phase Phase, message, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setPhaseLocked(phase, message, detail)
}

// logf is the test-facing log append.
func (s *Session) logf(kind LogKind, text, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logfLocked(kind, text, detail)
}

// logReset is the test-facing log reset.
func (s *Session) logReset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logResetLocked()
}

// needsManualVerification reports whether the error means the user has to
// solve the site's challenge in the browser panel.
func needsManualVerification(err error) bool {
	var sessionErr SessionError
	if errors.As(err, &sessionErr) {
		return true
	}
	message := err.Error()
	for _, token := range []string{"Cloudflare", "挑战页", "站点拦截", "完成验证", "站点 Cookie"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

// lastTitle is the title of the newest downloaded chapter.
func lastTitle(chapters []Chapter) string {
	if len(chapters) == 0 {
		return ""
	}
	return chapters[len(chapters)-1].Title
}

// lastOrder is the order of the newest downloaded chapter.
func lastOrder(chapters []Chapter) int {
	if len(chapters) == 0 {
		return 0
	}
	return chapters[len(chapters)-1].Order
}

// nextTitle is the title of the chapter at an index.
func nextTitle(catalog BookCatalog, next int) string {
	if next >= 0 && next < len(catalog.Chapters) {
		return catalog.Chapters[next].Title
	}
	if len(catalog.Chapters) > 0 {
		return catalog.Chapters[len(catalog.Chapters)-1].Title
	}
	return "未命名章节"
}

// nextOrder is the order of the chapter at an index.
func nextOrder(catalog BookCatalog, next int) int {
	if next >= 0 && next < len(catalog.Chapters) {
		return catalog.Chapters[next].Order
	}
	return max(len(catalog.Chapters), 1)
}
