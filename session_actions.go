package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// update runs fn on the UI thread with the session locked, then redraws.
//
// It is the one funnel for changing state from a goroutine: the lock keeps a
// test's polling (or a second goroutine) from reading a half-applied change,
// and the redraw happens after the change is complete. Without a window
// (tests) it runs inline.
func (s *Session) update(fn func()) {
	run := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		fn()
	}
	if s.win != nil {
		s.win.Update(run)
		return
	}
	run()
}

// invalidate asks for a frame without changing state.
func (s *Session) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// logfLocked appends a line to the run's log. The caller holds the lock.
func (s *Session) logfLocked(kind LogKind, text, detail string) {
	s.logSeq++
	s.log = append(s.log, LogLine{Seq: s.logSeq, At: time.Now(), Kind: kind, Text: text, Detail: detail})
	if len(s.log) > maxLogLines {
		s.log = s.log[len(s.log)-maxLogLines:]
	}
}

// logChapterLocked records a chapter the crawler took, with how long it took
// and how it was fetched. The caller holds the lock.
func (s *Session) logChapterLocked(order int, title string, transport Transport, took time.Duration) {
	detail := ""
	if transport == TransportBrowser {
		detail = "浏览器回退"
	}
	s.logSeq++
	s.log = append(s.log, LogLine{
		Seq: s.logSeq, At: time.Now(), Kind: LogChapter,
		Text: fmt.Sprintf("%03d  %s", order, title), Detail: detail, Took: took,
	})
	if len(s.log) > maxLogLines {
		s.log = s.log[len(s.log)-maxLogLines:]
	}
}

// logResetLocked clears the log for a new run. The caller holds the lock.
func (s *Session) logResetLocked() {
	s.log = nil
	s.logSeq = 0
}

// Submit is what the one input field does on Enter: a title searches, an
// address opens. It is the only entry point, so the flow the README promises
// is the flow the window offers.
func (s *Session) Submit() {
	s.mu.Lock()
	busy := s.phase.Busy()
	text := trimParagraph(s.input)
	s.mu.Unlock()
	if busy || text == "" {
		return
	}
	if s.looksLikeAddress(text) {
		s.openSource()
		return
	}
	s.search()
}

// looksLikeAddress reports whether the typed text names a page rather than a
// title. An address is anything with a scheme, or that belongs to one of the
// sites.
func (s *Session) looksLikeAddress(text string) bool {
	if strings.Contains(text, "://") {
		return true
	}
	for _, source := range Sources {
		if source.CatalogURL(text) != nil {
			return true
		}
	}
	return false
}

// openSource opens the address in the input and reads its catalog in one
// step. The browser only comes forward if the site demands a human.
func (s *Session) openSource() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openSourceLocked()
}

func (s *Session) openSourceLocked() {
	page := s.source.CatalogURL(s.input)
	if page == nil {
		// The address may belong to another site: switch to it and try again,
		// so a pasted link never answers "请输入 69 书吧的…".
		if adopted, ok := s.adoptForAddressLocked(s.input); ok {
			s.adoptLocked(adopted)
			page = s.source.CatalogURL(s.input)
		}
	}
	if page == nil {
		s.failLocked(errors.New(s.source.InvalidText))
		return
	}
	if s.phase.Busy() {
		return
	}

	s.input = page.String()

	// Reopening the same catalog to finish a verification must not throw away
	// the chapters already downloaded.
	if s.catalog != nil && s.catalog.SourceURL == page.String() {
		s.navSeq++
		s.panel.OnLoaded(nil)
		s.panel.Open(page.String())
		s.setPhaseLocked(PhaseCatalogReady,
			fmt.Sprintf("目录已保留，共 %d 章，已下载 %d 章。", len(s.catalog.Chapters), len(s.chapters)), "")
		return
	}

	s.resetLocked()
	s.logResetLocked()
	s.logfLocked(LogInfo, "打开目录页", page.String())

	// Read the catalog automatically once the page settles. The browser stays
	// out of the way unless the site asks for a human.
	s.navSeq++
	seq := s.navSeq
	s.setPhaseLocked(PhaseParsing, "正在读取目录…", "")
	s.panel.OnLoaded(func() { s.catalogPageLoaded(seq) })
	s.panel.Open(page.String())
}

// adoptForAddressLocked finds the site an address belongs to, when it is not
// the current one. The caller holds the lock.
func (s *Session) adoptForAddressLocked(text string) (Source, bool) {
	for _, candidate := range Sources {
		if candidate.Kind == s.source.Kind {
			continue
		}
		if candidate.CatalogURL(text) != nil {
			return candidate, true
		}
	}
	return Source{}, false
}

// catalogPageLoaded reads the catalog after the page settles.
//
// This is the guard that used to deadlock the app: it checked "is the session
// busy", and the phase that waits for exactly this callback counted as busy,
// so the load could never start. It now checks the navigation it belongs to
// and the phase it expects, and nothing else.
func (s *Session) catalogPageLoaded(seq int) {
	s.update(func() {
		if seq != s.navSeq {
			return // a page from an earlier attempt
		}
		if s.phase != PhaseParsing {
			return // the user moved on
		}
		if s.catalog != nil {
			return // already read
		}
		s.readCatalogLocked()
	})
}

// readCatalogLocked reads the catalog through the browser's cookies. The
// caller holds the lock.
func (s *Session) readCatalogLocked() {
	page := s.source.CatalogURL(s.input)
	if page == nil {
		s.failLocked(errors.New(s.source.InvalidText))
		return
	}

	source, panel := s.source, s.panel
	s.setPhaseLocked(PhaseParsing, "正在读取目录…", "")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		html, cookies, agent, err := panel.Sync(ctx)
		if err != nil {
			s.update(func() { s.catalogFailedLocked(err) })
			return
		}

		crawler := NewCrawler(source, agent, page.String(), cookies, panel.LoadHTML)
		catalog, err := crawler.FetchCatalog(ctx, page.String(), html)
		if err != nil {
			s.update(func() { s.catalogFailedLocked(err) })
			return
		}

		s.update(func() {
			if s.phase != PhaseParsing {
				return
			}
			s.keepEditableMetadataLocked(catalog)
			s.logfLocked(LogInfo, "目录已就绪", fmt.Sprintf("共 %d 章", len(s.catalog.Chapters)))
			s.setPhaseLocked(PhaseCatalogReady,
				fmt.Sprintf("目录已就绪，共 %d 章。", len(s.catalog.Chapters)), "")
		})
	}()
}

// catalogFailedLocked reports a catalog that could not be read. The caller
// holds the lock.
func (s *Session) catalogFailedLocked(err error) {
	if needsManualVerification(err) {
		s.logfLocked(LogWarn, "需要人工验证", err.Error())
		s.setPhaseLocked(PhaseFailed, "站点需要人工验证。", err.Error())
		return
	}
	s.logfLocked(LogError, "读取目录失败", err.Error())
	s.failLocked(err)
}

// keepEditableMetadataLocked carries the user's export settings over to a
// freshly parsed catalog. The caller holds the lock.
func (s *Session) keepEditableMetadataLocked(parsed BookCatalog) {
	if s.catalog != nil {
		if s.catalog.Author != "Unknown" && s.catalog.Author != "" {
			parsed.Author = s.catalog.Author
		}
		parsed.BodyFont = s.catalog.BodyFont
		parsed.FrontPageText = s.catalog.FrontPageText
		parsed.ShowSource = s.catalog.ShowSource
		parsed.CoverImage = s.catalog.CoverImage
		parsed.CoverType = s.catalog.CoverType
		parsed.CoverExt = s.catalog.CoverExt
	}
	if parsed.BodyFont == "" {
		parsed.BodyFont = FontSystem
	}
	if parsed.FrontPageText == "" {
		parsed.FrontPageText = defaultFrontPageBodyText
	}
	s.catalog = &parsed
}

// search runs the input as a title on the current site. It asks the site
// directly first, because two of the three answer a plain request; only when
// that is refused does the browser panel open so a human can get through.
func (s *Session) search() {
	s.mu.Lock()
	query := trimParagraph(s.input)
	if query == "" || s.phase.Busy() {
		s.mu.Unlock()
		return
	}
	if s.source.SearchURL == nil {
		s.failLocked(errors.New("这个站点还不支持搜索，请粘贴地址。"))
		s.mu.Unlock()
		return
	}

	s.hits = nil
	s.logResetLocked()
	s.logfLocked(LogInfo, "搜索", query)

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	s.cancel = cancel
	s.setPhaseLocked(PhaseSearching, fmt.Sprintf("正在 %s 搜索「%s」…", s.source.Name, query), "")

	source := s.source
	s.mu.Unlock()

	go func() {
		hits, blocked, err := searchDirectly(ctx, source, query)
		s.update(func() {
			if s.phase != PhaseSearching {
				return // cancelled, or replaced by a newer search
			}
			switch {
			case err != nil && ctx.Err() != nil:
				s.logfLocked(LogWarn, "搜索超时", err.Error())
				s.setPhaseLocked(PhaseIdle, "搜索超时。站点可能需要验证，请在浏览器面板里完成后再搜一次。", "")
			case err != nil:
				s.logfLocked(LogError, "搜索失败", err.Error())
				s.failLocked(err)
			case blocked:
				s.searchThroughBrowserLocked(query)
			default:
				s.finishSearchLocked(hits)
			}
		})
	}()
}

// searchTimeout bounds one search, so a site that never answers cannot leave
// the window waiting. It is a context deadline, not a sleeping goroutine: a
// second search cancels the first instead of being killed by it.
const searchTimeout = 45 * time.Second

// searchThroughBrowser runs the search in the panel, for a site that will not
// answer a plain request.
func (s *Session) searchThroughBrowser(query string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchThroughBrowserLocked(query)
}

func (s *Session) searchThroughBrowserLocked(query string) {
	s.setPhaseLocked(PhaseSearching, fmt.Sprintf("在浏览器里搜索「%s」…", query), "")
	s.panel.OnLoaded(func() { s.readSearchResults() })
	if s.source.Kind == SourceSto9 {
		s.panel.Open("https://sto9.com/")
		s.panel.SubmitSearchAfterLoad(query)
	} else {
		s.panel.Open(s.source.searchURL(query))
	}
	s.armSearchDeadline()
}

// armSearchDeadline ends a browser search that never answers.
//
// A site behind Cloudflare can leave the page loading forever: the load event
// never fires, so nothing reads the results and the window sits on "处理中…"
// with no way out. The deadline turns that into a message the user can act on
// (solve the check, then search again) instead of a spinner that never stops.
func (s *Session) armSearchDeadline() {
	time.AfterFunc(browserSearchTimeout, func() {
		s.update(func() {
			if s.phase != PhaseSearching {
				return // it answered, or the user moved on
			}
			s.panel.ClearLoaded()
			s.logfLocked(LogWarn, "搜索超时", "站点页面没有加载完")
			s.setPhaseLocked(PhaseFailed, "站点页面没有加载完，可能需要人工验证。",
				"在浏览器面板里完成验证后，再搜索一次。")
		})
	})
}

// browserSearchTimeout bounds a search that has to run in the browser. It is
// longer than searchTimeout because a human may have to solve a check.
const browserSearchTimeout = 90 * time.Second

// readSearchResults parses what the panel is showing into books.
func (s *Session) readSearchResults() {
	s.update(func() {
		if s.phase != PhaseSearching {
			return
		}
		html := s.panel.HTML()
		base := mustParseURL(s.panel.URL())
		s.finishSearchLocked(s.source.parseSearch(html, base))
	})
}

// finishSearch records the outcome. An empty result is a result: it says the
// site has no such book, which is not a request failure and must not open the
// browser.
func (s *Session) finishSearch(hits []SearchHit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishSearchLocked(hits)
}

func (s *Session) finishSearchLocked(hits []SearchHit) {
	s.panel.ClearLoaded()
	s.hits = hits
	if len(hits) == 0 {
		s.logfLocked(LogWarn, "没有结果", "换个关键词或换个站点。")
		s.setPhaseLocked(PhaseIdle, "没有找到这本书，换个关键词或换个站点。", "")
		return
	}
	s.logfLocked(LogInfo, "找到结果", fmt.Sprintf("%d 本", len(hits)))
	s.setPhaseLocked(PhaseIdle, "", "")
}

// pickHit opens a book the search found.
func (s *Session) pickHit(hit SearchHit) {
	s.mu.Lock()
	if s.phase.Busy() {
		s.mu.Unlock()
		return
	}
	s.hits = nil
	s.input = hit.URL
	s.openSourceLocked()
	s.mu.Unlock()
}

// cancelSearch drops the results and returns to the empty state.
func (s *Session) cancelSearch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.hits = nil
	s.input = ""
	s.setPhaseLocked(PhaseIdle, "", "")
}

// startDownload begins the book from its first chapter.
func (s *Session) startDownload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog == nil || s.phase.Busy() {
		return
	}
	s.chapters = nil
	s.next = 0
	s.recordID = ""
	s.launchDownloadLocked()
}

// resumeDownload continues from the chapter the run stopped on.
func (s *Session) resumeDownload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog == nil || s.phase.Busy() {
		return
	}
	s.launchDownloadLocked()
}

// skipChapter drops the chapter that failed and continues.
func (s *Session) skipChapter() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog == nil || s.phase.Busy() || s.next >= len(s.catalog.Chapters) {
		return
	}
	s.next++
	s.launchDownloadLocked()
}

// launchDownloadLocked starts the run from s.next, keeping what is already
// done. The caller holds the lock.
func (s *Session) launchDownloadLocked() {
	catalog := *s.catalog
	if catalog.BodyFont == "" {
		catalog.BodyFont = FontSystem
	}

	job := Job{
		Source:   s.source,
		Catalog:  catalog,
		Start:    s.next,
		Done:     slices.Clone(s.chapters),
		RecordID: s.recordID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.stopping.Store(false)
	s.startedAt = time.Now()
	s.startCount = len(job.Done)
	s.setPhaseLocked(PhaseDownloading, "", "")

	if job.Start == 0 {
		s.logResetLocked()
		s.logfLocked(LogInfo, "开始下载", fmt.Sprintf("%s · 共 %d 章", catalog.Title, len(catalog.Chapters)))
	} else {
		s.logfLocked(LogInfo, "继续下载", fmt.Sprintf("从第 %d 章开始，已有 %d 章", job.Start+1, len(job.Done)))
	}

	crawler := NewCrawler(s.source, s.panel.Agent(), catalog.SourceURL, s.panel.Cookies(), s.panel.LoadHTML)
	go s.runDownload(ctx, job, crawler)
}

// pauseDownload pauses a running download, keeping what it downloaded.
//
// The phase changes here, on the UI thread, not when the download goroutine
// notices the cancellation. Pausing is a user action: the button has to become
// "继续下载" the moment it is pressed. Waiting for the goroutine to report back
// left the window saying "暂停下载" over a download that had already stopped.
func (s *Session) pauseDownload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != PhaseDownloading {
		return
	}

	// Keep what is downloaded before the run is asked to stop, so the record
	// on disk matches the count the window shows.
	if s.catalog != nil && len(s.chapters) > 0 {
		s.recordID = s.saveHistoryLocked(*s.catalog, s.chapters, s.recordID)
	}

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	total := 0
	if s.catalog != nil {
		total = len(s.catalog.Chapters)
	}
	s.progress = &Progress{
		Completed: len(s.chapters), Total: total,
		CurrentTitle: nextTitle(*s.catalog, s.next), CurrentIndex: nextOrder(*s.catalog, s.next),
		LastTitle: lastTitle(s.chapters), LastIndex: lastOrder(s.chapters), HasLast: len(s.chapters) > 0,
	}
	s.logfLocked(LogWarn, "已暂停", fmt.Sprintf("保留 %d / %d 章", len(s.chapters), total))
	s.setPhaseLocked(PhasePaused, fmt.Sprintf("已暂停，保留 %d / %d 章。", len(s.chapters), total), "")
}

// stopDownload clears the download, keeping its history record.
func (s *Session) stopDownload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopping.Store(true)
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.catalog != nil && len(s.chapters) > 0 {
		s.saveHistoryLocked(*s.catalog, s.chapters, s.recordID)
	}
	s.chapters = nil
	s.next = 0
	s.recordID = ""
	s.progress = nil
	s.setPhaseLocked(PhaseStopped, "已停止，当前下载进度已清空。", "")
}

// saveHistoryLocked stores what is downloaded so far, and returns the record's
// ID. The caller holds the lock.
func (s *Session) saveHistoryLocked(catalog BookCatalog, chapters []Chapter, recordID string) string {
	if len(chapters) == 0 || s.history == nil {
		return recordID
	}
	settings := catalog.Settings()
	record := HistoryRecord{
		ID:            recordID,
		Title:         settings.Title,
		Author:        settings.Author,
		SourceURL:     settings.SourceURL,
		TotalChapters: settings.Total,
		Settings:      settings,
		Chapters:      slices.Clone(chapters),
	}
	id, err := s.history.Save(record)
	if err != nil {
		return recordID
	}
	return id
}

// removeChapter drops a downloaded chapter from the export.
func (s *Session) removeChapter(order int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase.Busy() {
		return
	}
	chapters := s.chapters[:0]
	for _, chapter := range s.chapters {
		if chapter.Order != order {
			chapters = append(chapters, chapter)
		}
	}
	s.chapters = chapters
	if s.catalog != nil {
		s.recordID = s.saveHistoryLocked(*s.catalog, s.chapters, s.recordID)
	}
}
