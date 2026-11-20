package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// app is JustNovel's state: the book, the download, and what the window shows.
type app struct {
	win     *mygo.Window
	session *browserSession
	history *History

	source      Source
	urlText     string
	showBrowser bool

	// search is the query, its results, and whether it is running. Searching
	// is how a book is found: the address field is only for pasting a link.
	query       string
	hits        []SearchHit
	searching   bool
	searchError string
	hitScroll   ui.ListState

	catalog *BookCatalog
	job     JobState

	chapters []Chapter
	next     int
	running  bool
	cancel   context.CancelFunc
	recordID string

	lastSaved   string
	awaiting    bool
	pendingStop bool

	// startedAt and startCount time the run, so the sheet can estimate what
	// is left from the pace it has actually kept.
	startedAt  time.Time
	startCount int

	// Draft of the export settings, edited in its dialog.
	showExport   bool
	draft        ExportSettings
	exportFont   string
	coverPreview ui.ImageSource

	showHistory    bool
	historyList    []HistoryRecord
	selectedID     string
	historyCover   ui.ImageSource
	historyCoverID string
	historyError   string
	pendingDelete  bool

	showChapters bool

	// log is the run's execution log, newest last: what the crawler did, in
	// the order it did it. It is the main surface of the window.
	log      []LogLine
	logSeq   int
	logState ui.ListState

	// catalogScroll keeps the chapter table on the chapter in flight, and
	// catalogScrollFor remembers which one it last followed.
	catalogScroll    ui.ListState
	catalogScrollFor int
	stopping atomic.Bool
}

func newApp() *app {
	source := SourceFor(SourceShuba69)
	return &app{
		source:      source,
		showBrowser: false,
		job:         JobState{Phase: PhaseIdle},
	}
}

// canParse reports whether the URL can be read.
func (a *app) canParse() bool {
	return a.source.CatalogURL(a.urlText) != nil && !a.job.Busy()
}

// canSync reports whether the browser has a settled page to read.
func (a *app) canSync() bool {
	if !a.canParse() || a.session == nil {
		return false
	}
	state := a.session.State()
	return state.Loaded && !state.Loading
}

// canPause reports whether the running download can be paused.
func (a *app) canPause() bool {
	return a.running && (a.job.Phase == PhaseDownloading || a.job.Phase == PhaseStarting)
}

// canResume reports whether a paused or failed download can continue.
func (a *app) canResume() bool {
	if a.running || a.catalog == nil {
		return false
	}
	if a.job.Phase != PhasePaused && a.job.Phase != PhaseFailed {
		return false
	}
	return a.next < len(a.catalog.Chapters)
}

// canExportSession reports whether the chapters downloaded so far can be
// exported.
func (a *app) canExportSession() bool {
	return len(a.chapters) > 0 && !a.job.Busy()
}

// selectSource switches the site, clearing the book. The address is kept when
// it already belongs to the new site; otherwise it is filled with that site's
// example, so switching never leaves an empty field to paste into again.
func (a *app) selectSource(kind SourceKind) {
	if kind == a.source.Kind || a.job.Busy() || a.running {
		return
	}
	a.source = SourceFor(kind)
	if a.source.CatalogURL(a.urlText) == nil {
		a.urlText = a.source.Example
	}
	a.session.Configure(a.source)
	a.resetSession()
}

// resetSession forgets the book and its chapters.
func (a *app) resetSession() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.running = false
	a.catalog = nil
	a.chapters = nil
	a.next = 0
	a.recordID = ""
	a.lastSaved = ""
	a.awaiting = false
	a.job = JobState{Phase: PhaseIdle}
}

// detectSource switches to the site the typed address belongs to, so a
// pasted catalog URL works without choosing the site first.
func (a *app) detectSource() {
	if strings.TrimSpace(a.urlText) == "" {
		return
	}
	if a.source.CatalogURL(a.urlText) != nil {
		return
	}
	for _, candidate := range Sources {
		if candidate.Kind == a.source.Kind {
			continue
		}
		if candidate.CatalogURL(a.urlText) != nil {
			a.adoptSource(candidate.Kind)
			return
		}
	}
}

// adoptSource switches sites but keeps the address the user typed.
func (a *app) adoptSource(kind SourceKind) {
	if kind == a.source.Kind {
		return
	}
	a.source = SourceFor(kind)
	a.session.Configure(a.source)
	a.resetSession()
}

// openSource opens the catalog and reads it in one step. The user pastes an
// address and presses Enter; the browser only appears if the site demands a
// human, and the catalog is read as soon as the page settles.
func (a *app) openSource() {
	page := a.source.CatalogURL(a.urlText)
	if page == nil {
		a.job = JobState{Phase: PhaseFailed, Message: a.source.InvalidText}
		return
	}
	if a.job.Busy() {
		return
	}
	a.urlText = page.String()
	// Reopening the same catalog to finish a verification must not throw
	// away the chapters already downloaded.
	if a.catalog != nil && a.catalog.SourceURL == page.String() {
		a.session.Open(page.String())
		if len(a.chapters) == 0 {
			a.job = JobState{Phase: PhaseCatalogReady, Message: fmt.Sprintf("目录已保留，共 %d 章。可重新同步或直接下载。", len(a.catalog.Chapters))}
		}
		return
	}
	a.resetSession()
	a.logReset()
	a.logf(LogInfo, "打开目录页", page.String())

	// Read the catalog automatically once the page settles. The browser stays
	// out of the way unless the site asks for a human.
	a.session.OnLoaded(a.autoSync)
	a.session.Open(page.String())
	a.job = JobState{Phase: PhaseParsing, Message: "正在读取目录…"}
}

// autoSync reads the catalog after the page loads, if it has not been read
// already. It is what makes one paste enough.
func (a *app) autoSync() {
	a.win.Update(func() {
		if a.job.Busy() || a.catalog != nil {
			return
		}
		if !a.canParse() {
			return
		}
		a.syncSession()
	})
}

// syncSession reads the catalog through the browser's cookies.
func (a *app) syncSession() {
	page := a.source.CatalogURL(a.urlText)
	if page == nil {
		a.job = JobState{Phase: PhaseFailed, Message: a.source.InvalidText}
		return
	}
	if a.job.Busy() {
		return
	}

	a.win.Update(func() { a.job = JobState{Phase: PhaseParsing} })
	source, session := a.source, a.session

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		html, cookies, agent, err := session.Sync(ctx)
		if err != nil {
			a.finishSyncErr(err)
			return
		}

		crawler := NewCrawler(source, agent, page.String(), cookies, session.LoadHTML)
		catalog, err := crawler.FetchCatalog(ctx, page.String(), html)
		if err != nil {
			a.finishSyncErr(err)
			return
		}
		a.win.Update(func() {
			a.keepEditableMetadata(catalog)
			a.job = JobState{Phase: PhaseCatalogReady, Message: fmt.Sprintf("目录已就绪，共 %d 章。确认后开始下载。", len(a.catalog.Chapters))}
			a.awaiting = false
		})
	}()
}

func (a *app) finishSyncErr(err error) {
	a.win.Update(func() {
		var sessionErr SessionError
		if errors.As(err, &sessionErr) {
			// The site wants a human: open the browser for them.
			a.showBrowser = true
			a.awaiting = true
			a.logf(LogWarn, "需要人工验证", sessionErr.Message)
			a.job = JobState{Phase: PhaseSessionRequired, Message: sessionErr.Message}
		} else {
			a.logf(LogError, "同步失败", err.Error())
			a.job = JobState{Phase: PhaseFailed, Message: err.Error()}
		}
	})
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

// keepEditableMetadata carries the user's export settings over to a freshly
// parsed catalog.
func (a *app) keepEditableMetadata(parsed BookCatalog) {
	if a.catalog != nil && a.catalog.Author != "Unknown" {
		parsed.Author = a.catalog.Author
	}
	if a.catalog != nil {
		parsed.BodyFont = a.catalog.BodyFont
		parsed.FrontPageText = a.catalog.FrontPageText
		parsed.ShowSource = a.catalog.ShowSource
		parsed.CoverImage = a.catalog.CoverImage
		parsed.CoverType = a.catalog.CoverType
		parsed.CoverExt = a.catalog.CoverExt
	}
	if parsed.BodyFont == "" {
		parsed.BodyFont = FontSystem
	}
	if parsed.FrontPageText == "" {
		parsed.FrontPageText = defaultFrontPageBodyText
	}
	a.catalog = &parsed
}

func (a *app) startDownload() {
	if a.catalog == nil {
		a.job = JobState{Phase: PhaseFailed, Message: "请先解析目录，再开始下载。"}
		return
	}
	a.chapters = nil
	a.next = 0
	a.recordID = ""
	a.launchDownload()
}

func (a *app) resumeDownload() {
	a.launchDownload()
}

func (a *app) launchDownload() {
	catalog := *a.catalog
	if catalog.BodyFont == "" {
		catalog.BodyFont = FontSystem
	}
	start := a.next
	done := slices.Clone(a.chapters)

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.stopping.Store(false)
	a.startedAt = time.Now()
	a.startCount = len(done)
	a.job = JobState{Phase: PhaseStarting}

	if start == 0 {
		a.logReset()
		a.logf(LogInfo, "开始下载", fmt.Sprintf("%s · 共 %d 章", catalog.Title, len(catalog.Chapters)))
	} else {
		a.logf(LogInfo, "继续下载", fmt.Sprintf("从第 %d 章开始，已有 %d 章", start+1, len(done)))
	}

	crawler := NewCrawler(a.source, a.session.Agent(), catalog.SourceURL, a.session.Cookies(), a.session.LoadHTML)

	go a.downloadLoop(ctx, crawler, catalog, start, done)
}

func (a *app) downloadLoop(ctx context.Context, crawler *Crawler, catalog BookCatalog, start int, done []Chapter) {
	total := len(catalog.Chapters)
	recordID := a.recordID
	transport := TransportSession

	for i := start; i < total; i++ {
		if a.stopping.Load() {
			return
		}
		chapter := catalog.Chapters[i]
		chapterStart := time.Now()
		a.win.Update(func() {
			if a.stopping.Load() {
				return
			}
			a.job = JobState{Phase: PhaseDownloading, Snap: &Progress{
				Completed: len(done), Total: total,
				CurrentTitle: chapter.Title, CurrentIndex: chapter.Order,
				Transport: transport,
				LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
			}}
		})

		content, used, err := crawler.FetchChapter(ctx, chapter, func(t Transport) {
			transport = t
			a.win.Update(func() {
				if a.stopping.Load() {
					return
				}
				if t == TransportBrowser {
					a.logf(LogFallback, "切换到浏览器抓取", fmt.Sprintf("%03d  %s", chapter.Order, chapter.Title))
				}
				a.job = JobState{Phase: PhaseDownloading, Snap: &Progress{
					Completed: len(done), Total: total,
					CurrentTitle: chapter.Title, CurrentIndex: chapter.Order,
					Transport: t,
					LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
				}}
			})
		})
		if err != nil {
			if ctx.Err() != nil {
				if a.stopping.Load() {
					return
				}
				a.pause(done, i, transport, catalog, recordID)
				return
			}
			a.failDownload(err, done, i, transport, catalog, recordID)
			return
		}
		if a.stopping.Load() {
			return
		}
		transport = used
		done = append(done, content)

		if len(done)%50 == 0 || len(done) == total {
			recordID = a.saveHistory(catalog, done, recordID)
		}

		completed := len(done)
		a.win.Update(func() {
			if a.stopping.Load() {
				return
			}
			a.chapters = slices.Clone(done)
			a.next = i + 1
			a.recordID = recordID
			a.awaiting = false
			a.logChapter(chapter.Order, chapter.Title, transport, time.Since(chapterStart))
			a.job = JobState{Phase: PhaseDownloading, Snap: &Progress{
				Completed: completed, Total: total,
				CurrentTitle: chapter.Title, CurrentIndex: chapter.Order,
				Transport: transport,
				LastTitle: chapter.Title, LastIndex: chapter.Order, HasLast: true,
			}}
		})

		if i+1 < total {
			if err := crawler.WaitBetweenChapters(ctx); err != nil {
				if a.stopping.Load() {
					return
				}
				a.pause(done, i+1, transport, catalog, recordID)
				return
			}
		}
	}

	recordID = a.saveHistory(catalog, done, recordID)
	if a.stopping.Load() {
		return
	}
	if len(done) < total {
		a.win.Update(func() {
			if a.stopping.Load() {
				return
			}
			a.running = false
			a.chapters = slices.Clone(done)
			a.next = total
			a.recordID = recordID
			if len(done) == 0 {
				a.job = JobState{Phase: PhaseFinished, Message: "没有成功下载的章节，未生成 EPUB。"}
				return
			}
			a.job = JobState{Phase: PhaseFinished, Message: fmt.Sprintf("已完成 %d / %d 章（跳过 %d 章），正在生成 EPUB。", len(done), total, total-len(done))}
		})
		if a.stopping.Load() || len(done) == 0 {
			return
		}
		a.beginExport(catalog.Settings(), slices.Clone(done), recordID, JobState{Phase: PhaseFinished, Message: "EPUB 导出成功。"}, false)
		return
	}
	a.win.Update(func() {
		if a.stopping.Load() {
			return
		}
		a.running = false
		a.chapters = slices.Clone(done)
		a.next = total
		a.recordID = recordID
		a.job = JobState{Phase: PhaseFinished, Message: fmt.Sprintf("已下载全部 %d 章，正在生成 EPUB。", total)}
	})
	if a.stopping.Load() {
		return
	}
	a.beginExport(catalog.Settings(), slices.Clone(done), recordID, JobState{Phase: PhaseFinished, Message: "EPUB 导出成功。"}, false)
}

func (a *app) pause(done []Chapter, next int, transport Transport, catalog BookCatalog, recordID string) {
	recordID = a.saveHistory(catalog, done, recordID)
	a.win.Update(func() {
		a.running = false
		a.chapters = slices.Clone(done)
		a.next = next
		a.recordID = recordID
		a.cancel = nil
		a.logf(LogWarn, "已暂停", fmt.Sprintf("保留 %d / %d 章", len(done), len(catalog.Chapters)))
		a.job = JobState{Phase: PhasePaused, Snap: &Progress{
			Completed: len(done), Total: len(catalog.Chapters),
			CurrentTitle: nextTitle(catalog, next), CurrentIndex: nextOrder(catalog, next),
			Transport: transport,
			LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
		}}
	})
}

func (a *app) failDownload(err error, done []Chapter, next int, transport Transport, catalog BookCatalog, recordID string) {
	recordID = a.saveHistory(catalog, done, recordID)
	message := err.Error()
	a.win.Update(func() {
		a.running = false
		a.chapters = slices.Clone(done)
		a.next = next
		a.recordID = recordID
		a.cancel = nil

		if needsManualVerification(err) {
			a.awaiting = true
		}
		a.logf(LogError, "下载中断", message)
		a.job = JobState{Phase: PhaseFailed,
			Message: "下载失败。你可以继续当前下载、跳过当前章、从头重下，或先导出已完成内容。\n" + message,
			Snap: &Progress{
				Completed: len(done), Total: len(catalog.Chapters),
				CurrentTitle: nextTitle(catalog, next), CurrentIndex: nextOrder(catalog, next),
				Transport: transport,
				LastTitle: lastTitle(done), LastIndex: lastOrder(done), HasLast: len(done) > 0,
			}}
	})
}

// restartDownload discards the current run and starts the book again.
func (a *app) restartDownload() {
	if a.catalog == nil || a.running {
		return
	}
	a.startDownload()
}

// pauseDownload pauses a running download, keeping what it downloaded.
func (a *app) pauseDownload() {
	if !a.canPause() || a.cancel == nil {
		return
	}
	a.cancel()
}

// skipChapter drops the chapter that failed and continues.
func (a *app) skipChapter() {
	if a.running || a.catalog == nil || a.next >= len(a.catalog.Chapters) {
		return
	}
	a.next++
	a.launchDownload()
}

// stopDownload clears the download, keeping its history record.
func (a *app) stopDownload() {
	a.stopping.Store(true)
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.running = false
	if a.catalog != nil && len(a.chapters) > 0 {
		a.saveHistory(*a.catalog, a.chapters, a.recordID)
	}
	a.chapters = nil
	a.next = 0
	a.recordID = ""
	a.job = JobState{Phase: PhaseStopped}
}

// saveHistory stores what is downloaded so far, and returns the record's ID.
func (a *app) saveHistory(catalog BookCatalog, chapters []Chapter, recordID string) string {
	if len(chapters) == 0 || a.history == nil {
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
	id, err := a.history.Save(record)
	if err != nil {
		return recordID
	}
	return id
}

// openExportDialog opens the export settings of the current session.
func (a *app) openExportDialog() {
	if a.catalog == nil || len(a.chapters) == 0 {
		return
	}
	a.draft = a.catalog.Settings()
	if trimParagraph(a.draft.FrontText) == "" {
		a.draft.FrontText = defaultFrontPageBodyText
	}
	a.exportFont = a.draft.BodyFont.DisplayName()
	a.showExport = true
	a.refreshCoverPreview()
}

func (a *app) refreshCoverPreview() {
	a.coverPreview = nil
	if len(a.draft.CoverImage) == 0 {
		return
	}
	if bitmap, err := ui.DecodeBitmap(a.draft.CoverImage); err == nil {
		a.coverPreview = bitmap
	}
}

// confirmExport applies the edited settings and writes the book.
func (a *app) confirmExport() {
	if a.catalog == nil {
		return
	}
	for _, preset := range FontPresets {
		if preset.DisplayName() == a.exportFont {
			a.draft.BodyFont = preset
		}
	}
	if author := trimParagraph(a.draft.Author); author != "" {
		a.draft.Author = author
	} else {
		a.draft.Author = "Unknown"
	}
	if a.draft.BodyFont == "" {
		a.draft.BodyFont = FontSystem
	}

	catalog := *a.catalog
	catalog.ApplySettings(a.draft)
	a.catalog = &catalog
	a.showExport = false

	a.beginExport(catalog.Settings(), slices.Clone(a.chapters), a.recordID,
		JobState{Phase: PhaseFinished, Message: "EPUB 导出成功。"}, true)
}

// importCover asks for an image and makes it the book's cover.
func (a *app) importCover() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:   "选择封面",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "webp"}, MIMETypes: []string{"image/*"}}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	if err != nil || len(data) == 0 {
		return
	}
	mediaType, ext := coverTypeFor(paths[0])
	a.draft.CoverImage = data
	a.draft.CoverType = mediaType
	a.draft.CoverExt = ext
	a.refreshCoverPreview()
}

// removeCover clears the draft's cover.
func (a *app) removeCover() {
	a.draft.CoverImage = nil
	a.draft.CoverType = ""
	a.draft.CoverExt = ""
	a.coverPreview = nil
}

// beginExport builds the EPUB and writes it. A finished download saves
// straight to Downloads; a manual export asks where to put the file.
func (a *app) beginExport(settings ExportSettings, chapters []Chapter, recordID string, after JobState, askPath bool) {
	if len(chapters) == 0 {
		a.win.Update(func() { a.job = JobState{Phase: PhaseFailed, Message: "当前还没有可导出的章节。"} })
		return
	}
	a.win.Update(func() { a.job = JobState{Phase: PhaseBuilding} })
	a.win.Update(func() { a.logf(LogInfo, "开始生成 EPUB", fmt.Sprintf("%d 章", len(chapters))) })
	name := ExportFileName(settings.Title, len(chapters), settings.Total)

	go func() {
		data, err := BuildEPUB(settings, chapters)
		if err != nil {
			a.win.Update(func() { a.job = JobState{Phase: PhaseFailed, Message: "EPUB 生成失败：" + err.Error()} })
			return
		}

		path := uniqueDownloadPath(name)
		if askPath {
			chosen, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
				Title:       "导出 EPUB",
				DefaultPath: path,
				Filters:     []mygo.FileFilter{{Name: "EPUB", Extensions: []string{"epub"}}},
			})
			if err != nil {
				a.win.Update(func() { a.job = JobState{Phase: PhaseFailed, Message: "保存失败：" + err.Error()} })
				return
			}
			if chosen == "" {
				a.win.Update(func() {
					a.job = JobState{Phase: PhaseFinished, Message: "已取消导出。"}
				})
				return
			}
			path = chosen
		}
		if !strings.HasSuffix(strings.ToLower(path), ".epub") {
			path += ".epub"
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			a.win.Update(func() { a.job = JobState{Phase: PhaseFailed, Message: "保存失败：" + err.Error()} })
			return
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			a.win.Update(func() { a.job = JobState{Phase: PhaseFailed, Message: "保存失败：" + err.Error()} })
			return
		}
		if recordID != "" && a.history != nil {
			a.history.MarkExported(recordID, filepath.Base(path))
		}

		a.win.Update(func() {
			a.lastSaved = path
			a.logf(LogDone, "EPUB 已导出", filepath.Base(path))
			if askPath {
				a.job = after
			} else {
				a.job = JobState{Phase: PhaseFinished, Message: "EPUB 已导出：" + filepath.Base(path)}
			}
		})
	}()
}

// uniqueDownloadPath picks a Downloads path that does not overwrite an
// earlier export.
func uniqueDownloadPath(name string) string {
	dir := downloadsDir()
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	path := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); err != nil {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
	}
}

func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// openHistory loads the saved downloads into the dialog.
func (a *app) openHistory() {
	a.historyList = a.history.List()
	a.showHistory = true
	a.historyError = ""
	if a.selectedID == "" || !a.hasRecord(a.selectedID) {
		a.selectedID = ""
		if len(a.historyList) > 0 {
			a.selectedID = a.historyList[0].ID
		}
	}
	a.selectHistory(a.selectedID)
}

// selectHistory chooses a record and decodes its cover once.
func (a *app) selectHistory(id string) {
	a.selectedID = id
	if id == a.historyCoverID {
		return
	}
	a.historyCoverID = id
	a.historyCover = nil
	record, ok := a.selectedRecord()
	if !ok || len(record.Settings.CoverImage) == 0 {
		return
	}
	if bitmap, err := ui.DecodeBitmap(record.Settings.CoverImage); err == nil {
		a.historyCover = bitmap
	}
}

func (a *app) hasRecord(id string) bool {
	for _, record := range a.historyList {
		if record.ID == id {
			return true
		}
	}
	return false
}

func (a *app) selectedRecord() (HistoryRecord, bool) {
	for _, record := range a.historyList {
		if record.ID == a.selectedID {
			return record, true
		}
	}
	return HistoryRecord{}, false
}

// exportHistory writes a saved record again.
func (a *app) exportHistory() {
	record, ok := a.selectedRecord()
	if !ok || len(record.Chapters) == 0 {
		return
	}
	settings := record.Settings
	if settings.Total == 0 {
		settings.Total = record.TotalChapters
	}
	a.showHistory = false
	a.beginExport(settings, record.Chapters, record.ID,
		JobState{Phase: PhaseFinished, Message: "历史 EPUB 导出成功。"}, true)
}

// deleteHistory removes the selected record.
func (a *app) deleteHistory() {
	if a.selectedID == "" {
		return
	}
	if err := a.history.Delete(a.selectedID); err != nil {
		a.historyError = "删除历史记录失败：" + err.Error()
		return
	}
	a.historyList = a.history.List()
	a.selectedID = ""
	if len(a.historyList) > 0 {
		a.selectedID = a.historyList[0].ID
	}
	a.selectHistory(a.selectedID)
	a.pendingDelete = false
}

// removeChapter drops a downloaded chapter from the export.
func (a *app) removeChapter(order int) {
	if a.job.Busy() {
		return
	}
	chapters := a.chapters[:0]
	for _, chapter := range a.chapters {
		if chapter.Order != order {
			chapters = append(chapters, chapter)
		}
	}
	a.chapters = chapters
	if a.catalog != nil {
		a.recordID = a.saveHistory(*a.catalog, a.chapters, a.recordID)
	}
	if len(a.chapters) == 0 {
		a.showChapters = false
	}
}

// statusDetail is the explanation under the status headline.
func (a *app) statusDetail() string {
	switch a.job.Phase {
	case PhaseIdle:
		return a.source.IdleDetail
	case PhaseCatalogReady:
		return a.job.Message
	case PhaseSessionRequired:
		return a.job.Message
	case PhaseParsing:
		return "会话已同步，正在读取目录和章节信息。"
	case PhaseStarting:
		return "已开始下载，准备请求第一章。"
	case PhaseDownloading:
		if a.job.Snap == nil {
			return "正在下载章节。"
		}
		snap := a.job.Snap
		text := fmt.Sprintf("已完成 %d / %d 章。\n正在处理：第 %d 章《%s》\n当前方式：%s",
			snap.Completed, snap.Total, snap.CurrentIndex, snap.CurrentTitle, snap.Transport.Title())
		if snap.HasLast {
			text += fmt.Sprintf("\n最近完成：第 %d 章《%s》", snap.LastIndex, snap.LastTitle)
		}
		return text + "\n" + snap.Transport.Detail()
	case PhasePaused:
		if a.job.Snap == nil {
			return "已暂停，当前会话进度已保留。"
		}
		return fmt.Sprintf("已暂停，当前会话进度已保留。\n当前进度：已完成 %d / %d 章。\n你可以继续下载、导出当前内容，或停止并清空这次进度。",
			a.job.Snap.Completed, a.job.Snap.Total)
	case PhaseBuilding:
		return "正在写入目录、章节 XHTML 和 OPF 文件。"
	case PhaseFinished:
		return a.job.Message
	case PhaseStopped:
		return "已停止，当前下载进度已清空。"
	case PhaseFailed:
		return a.job.Message
	}
	return ""
}

func lastTitle(chapters []Chapter) string {
	if len(chapters) == 0 {
		return ""
	}
	return chapters[len(chapters)-1].Title
}

func lastOrder(chapters []Chapter) int {
	if len(chapters) == 0 {
		return 0
	}
	return chapters[len(chapters)-1].Order
}

func nextTitle(catalog BookCatalog, next int) string {
	if next >= 0 && next < len(catalog.Chapters) {
		return catalog.Chapters[next].Title
	}
	if len(catalog.Chapters) > 0 {
		return catalog.Chapters[len(catalog.Chapters)-1].Title
	}
	return "未命名章节"
}

func nextOrder(catalog BookCatalog, next int) int {
	if next >= 0 && next < len(catalog.Chapters) {
		return catalog.Chapters[next].Order
	}
	return max(len(catalog.Chapters), 1)
}
