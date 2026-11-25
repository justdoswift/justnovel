package main

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// Job is one download run: everything it needs, by value, so the loop never
// reads a field the UI thread may be changing underneath it.
type Job struct {
	Source   Source
	Catalog  BookCatalog
	Start    int
	Done     []Chapter
	RecordID string
}

// Total is how many chapters the book has.
func (j Job) Total() int { return len(j.Catalog.Chapters) }

// checkpointEvery is how often a run writes its progress to the history: every
// this many chapters, or every checkpointInterval, whichever comes first. A
// long book therefore never rewrites the whole record on every chapter, and a
// short one still saves promptly.
const (
	checkpointEvery    = 50
	checkpointInterval = 2 * time.Minute
)

// runDownload is the download loop. It owns no UI state: it reports through
// update, and every decision it makes is about the job it was handed.
func (s *Session) runDownload(ctx context.Context, job Job, crawler *Crawler) {
	total := job.Total()
	done := slices.Clone(job.Done)
	recordID := job.RecordID
	transport := TransportSession
	lastCheckpoint := time.Now()
	sinceCheckpoint := 0

	checkpoint := func() {
		s.mu.Lock()
		recordID = s.saveHistoryLocked(job.Catalog, done, recordID)
		s.mu.Unlock()
		lastCheckpoint = time.Now()
		sinceCheckpoint = 0
	}

	for i := job.Start; i < total; i++ {
		if s.stopped() {
			return
		}
		chapter := job.Catalog.Chapters[i]
		chapterStart := time.Now()
		s.reportProgress(done, total, chapter, transport)

		content, used, err := crawler.FetchChapter(ctx, chapter, func(t Transport) {
			transport = t
			if t == TransportBrowser {
				s.update(func() {
					if s.stopping.Load() {
						return
					}
					s.logfLocked(LogFallback, "切换到浏览器抓取", fmt.Sprintf("%03d  %s", chapter.Order, chapter.Title))
				})
			}
			s.reportProgress(done, total, chapter, t)
		})
		if err != nil {
			if ctx.Err() != nil {
				if s.stopped() {
					return
				}
				s.pause(done, i, transport, job.Catalog, recordID)
				return
			}
			s.failDownload(err, done, i, transport, job.Catalog, recordID)
			return
		}
		if s.stopped() {
			return
		}

		transport = used
		done = append(done, content)
		sinceCheckpoint++

		if sinceCheckpoint >= checkpointEvery || time.Since(lastCheckpoint) >= checkpointInterval || len(done) == total {
			checkpoint()
		}

		completed := len(done)
		took := time.Since(chapterStart)
		s.update(func() {
			if s.stopping.Load() || s.phase != PhaseDownloading {
				return
			}
			s.chapters = slices.Clone(done)
			s.next = i + 1
			s.recordID = recordID
			s.logChapterLocked(chapter.Order, chapter.Title, transport, took)
			s.progress = &Progress{
				Completed: completed, Total: total,
				CurrentTitle: chapter.Title, CurrentIndex: chapter.Order,
				Transport: transport,
				LastTitle: chapter.Title, LastIndex: chapter.Order, HasLast: true,
			}
		})

		if i+1 < total {
			if err := crawler.WaitBetweenChapters(ctx); err != nil {
				if s.stopped() {
					return
				}
				s.pause(done, i+1, transport, job.Catalog, recordID)
				return
			}
		}
	}

	checkpoint()
	if s.stopped() {
		return
	}

	skipped := total - len(done)
	message := fmt.Sprintf("已下载全部 %d 章，正在生成 EPUB。", total)
	if skipped > 0 {
		message = fmt.Sprintf("已完成 %d / %d 章（跳过 %d 章），正在生成 EPUB。", len(done), total, skipped)
	}

	s.update(func() {
		if s.stopping.Load() {
			return
		}
		s.chapters = slices.Clone(done)
		s.next = total
		s.recordID = recordID
		s.progress = &Progress{
			Completed: len(done), Total: total,
			Transport: transport,
			LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
		}
		s.setPhaseLocked(PhaseBuilding, message, "")
	})

	if len(done) == 0 {
		s.update(func() {
			if s.stopping.Load() {
				return
			}
			s.setPhaseLocked(PhaseFinished, "没有成功下载的章节，未生成 EPUB。", "")
		})
		return
	}
	s.beginExport(job.Catalog.Settings(), slices.Clone(done), recordID, false)
}

// reportProgress publishes the chapter in flight.
func (s *Session) reportProgress(done []Chapter, total int, chapter ChapterRef, transport Transport) {
	completed := len(done)
	s.update(func() {
		if s.stopping.Load() {
			return
		}
		s.progress = &Progress{
			Completed: completed, Total: total,
			CurrentTitle: chapter.Title, CurrentIndex: chapter.Order,
			Transport: transport,
			LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
		}
	})
}

// stopped reports whether the run was cancelled on purpose.
func (s *Session) stopped() bool {
	return s.stopping.Load()
}

// pause keeps what the run downloaded and stops on the next chapter.
//
// It only reports when the session is still downloading: a pause the user
// pressed has already set the phase and saved the record on the UI thread, and
// this must not overwrite the newer count with the goroutine's older one.
func (s *Session) pause(done []Chapter, next int, transport Transport, catalog BookCatalog, recordID string) {
	s.update(func() {
		if s.phase != PhaseDownloading {
			return
		}
		recordID = s.saveHistoryLocked(catalog, done, recordID)
		s.chapters = slices.Clone(done)
		s.next = next
		s.recordID = recordID
		s.logfLocked(LogWarn, "已暂停", fmt.Sprintf("保留 %d / %d 章", len(done), len(catalog.Chapters)))
		s.progress = &Progress{
			Completed: len(done), Total: len(catalog.Chapters),
			CurrentTitle: nextTitle(catalog, next), CurrentIndex: nextOrder(catalog, next),
			Transport: transport,
			LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
		}
		s.setPhaseLocked(PhasePaused, fmt.Sprintf("已暂停，保留 %d / %d 章。", len(done), len(catalog.Chapters)), "")
	})
}

// failDownload records why the run stopped and what can be done about it.
func (s *Session) failDownload(err error, done []Chapter, next int, transport Transport, catalog BookCatalog, recordID string) {
	s.update(func() {
		recordID = s.saveHistoryLocked(catalog, done, recordID)
		s.chapters = slices.Clone(done)
		s.next = next
		s.recordID = recordID
		s.lastErr = err
		s.logfLocked(LogError, "下载中断", err.Error())
		s.progress = &Progress{
			Completed: len(done), Total: len(catalog.Chapters),
			CurrentTitle: nextTitle(catalog, next), CurrentIndex: nextOrder(catalog, next),
			Transport: transport,
			LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
		}
		s.setPhaseLocked(PhaseFailed, "下载中断。", err.Error())
	})
}

