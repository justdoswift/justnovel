package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// testApp builds a window over a session whose browser is a scripted page, so
// every state the view draws can be reached without a real web view.
func testApp(t *testing.T) *app {
	t.Helper()
	return testAppWithPanel(t, &fakePage{})
}

// testAppWithPanel builds a window whose session has a panel backed by a
// scripted page, so the catalog flow can be exercised without a real browser.
func testAppWithPanel(t *testing.T, page *fakePage) *app {
	t.Helper()
	history := NewHistory(filepath.Join(t.TempDir(), "history"))
	panel := newFakePanel(page, SourceFor(SourceShuba69))
	session := NewSession(nil, panel, history, SourceFor(SourceShuba69))
	return newApp(session, panel, history)
}

func TestWindowShowsTheInputAndTheSite(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 820)

	for _, want := range []string{"justnovel", "搜一本书", "69 书吧", "输入书名，回车。也可以粘贴书籍页或目录页地址。"} {
		if !tt.HasText(want) {
			t.Errorf("window does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if *a.session.Input() != "" {
		t.Errorf("input = %q, want it empty", *a.session.Input())
	}
}

func TestInvalidAddressShowsAnError(t *testing.T) {
	a := testApp(t)

	*a.session.Input() = "https://example.com/not-a-catalog"
	a.session.Submit()

	if a.session.Phase() != PhaseFailed {
		t.Fatalf("phase = %v, want failed for an invalid address", a.session.Phase())
	}
	if a.session.Message() != a.session.Source().InvalidText {
		t.Errorf("message = %q, want %q", a.session.Message(), a.session.Source().InvalidText)
	}
}

// TestCatalogLoadsWithoutASecondPress is the regression for the deadlock that
// made 0.1.1 unable to finish a download: the catalog load waited for a page
// event, and the guard on that event asked whether the app was busy, while the
// phase that waits for the event counted as busy.
func TestCatalogLoadsWithoutASecondPress(t *testing.T) {
	page := &fakePage{
		html: `<html><head>
			<meta property="og:novel:book_name" content="极道天魔"/>
			<meta property="og:novel:author" content="滚开"/>
			</head><body><ul>
			<li><a href="/txt/90028/1">第一章 夜路</a></li>
			<li><a href="/txt/90028/2">第二章 敲门</a></li>
			</ul></body></html>`,
	}
	a := testAppWithPanel(t, page)

	*a.session.Input() = "https://www.69shuba.com/book/90028/"
	a.session.Submit()

	if a.session.Phase() != PhaseParsing {
		t.Fatalf("phase = %v, want parsing right after opening", a.session.Phase())
	}

	// The page settles. Nothing else is pressed.
	page.finishLoad()
	waitFor(t, func() bool { return a.session.Phase() == PhaseCatalogReady })

	catalog := a.session.Catalog()
	if catalog == nil {
		t.Fatal("the catalog was not read")
	}
	if catalog.Title != "极道天魔" || len(catalog.Chapters) != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}
}

// TestStalePageDoesNotReadTheCatalog checks that a page from an earlier
// navigation cannot read a catalog the user no longer wants.
func TestStalePageDoesNotReadTheCatalog(t *testing.T) {
	page := &fakePage{html: `<html><body><ul><li><a href="/txt/90028/1">第一章</a></li></ul></body></html>`}
	a := testAppWithPanel(t, page)

	*a.session.Input() = "https://www.69shuba.com/book/90028/"
	a.session.Submit()
	stale := a.session.navSeq

	// The user opens another book before the first page settles.
	a.session.reset()
	*a.session.Input() = "https://www.69shuba.com/book/1/"
	a.session.Submit()

	a.session.catalogPageLoaded(stale)
	if a.session.Catalog() != nil {
		t.Error("a stale page read a catalog")
	}
}

func TestReopeningTheSameCatalogKeepsProgress(t *testing.T) {
	page := &fakePage{html: `<html></html>`}
	a := testAppWithPanel(t, page)
	*a.session.Input() = "https://www.69shuba.com/book/90028/"
	a.session.catalog = &BookCatalog{
		Title:     "一本书",
		SourceURL: "https://www.69shuba.com/book/90028/",
		Chapters:  []ChapterRef{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}},
	}
	a.session.chapters = []Chapter{{Title: "第一章", Order: 1}}
	a.session.next = 1

	a.session.Submit()

	if len(a.session.Chapters()) != 1 || a.session.Catalog() == nil || a.session.Catalog().Title != "一本书" {
		t.Fatalf("reopening the catalog lost the download: chapters=%d catalog=%v",
			len(a.session.Chapters()), a.session.Catalog())
	}
	if a.session.Phase() != PhaseCatalogReady {
		t.Errorf("phase = %v, want the catalog kept", a.session.Phase())
	}
}

func TestOpeningAChapterPageNormalizesToTheCatalog(t *testing.T) {
	page := &fakePage{html: `<html></html>`}
	a := testAppWithPanel(t, page)
	a.session.adopt(SourceFor(SourcePiaotian))
	*a.session.Input() = "https://www.piaotia.com/html/15/15709/11428790.html"

	a.session.Submit()

	if *a.session.Input() != "https://www.piaotia.com/html/15/15709/" {
		t.Errorf("address = %q, want the catalog page", *a.session.Input())
	}
	if a.session.Phase() != PhaseParsing {
		t.Errorf("phase = %v, want parsing right after opening", a.session.Phase())
	}
}

// TestPastedAddressSwitchesTheSite checks a link from another site does not
// answer "请输入 69 书吧的…".
func TestPastedAddressSwitchesTheSite(t *testing.T) {
	page := &fakePage{html: `<html></html>`}
	a := testAppWithPanel(t, page)

	*a.session.Input() = "https://www.piaotia.com/html/15/15709/"
	a.session.Submit()

	if a.session.Source().Kind != SourcePiaotian {
		t.Fatalf("source = %v, want piaotian", a.session.Source().Kind)
	}
	if a.session.Phase() == PhaseFailed {
		t.Fatalf("a valid piaotian link failed: %q", a.session.Message())
	}
}

func TestStopDownloadKeepsTheHistoryRecord(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{
		Title:     "一本书",
		Author:    "作者",
		SourceURL: "https://www.69shuba.com/book/1/",
		Chapters:  []ChapterRef{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}},
	}
	a.session.chapters = []Chapter{{Title: "第一章", Order: 1, PlainText: "正文"}}
	a.session.setPhase(PhaseDownloading, "", "")

	a.session.stopDownload()

	if a.history.Count() != 1 {
		t.Fatalf("history records = %d, want the stopped download saved", a.history.Count())
	}
	if len(a.session.Chapters()) != 0 || a.session.Phase() != PhaseStopped {
		t.Fatalf("stop left chapters=%d phase=%v", len(a.session.Chapters()), a.session.Phase())
	}
}

func TestSourceSwitchResetsTheBook(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{Title: "一本书", Chapters: []ChapterRef{{Title: "第一章", Order: 1}}}
	a.session.chapters = []Chapter{{Title: "第一章", Order: 1}}
	a.session.next = 1

	a.session.adopt(SourceFor(SourcePiaotian))

	if a.session.Source().Kind != SourcePiaotian {
		t.Errorf("source = %v", a.session.Source().Kind)
	}
	if a.session.Catalog() != nil || len(a.session.Chapters()) != 0 || a.session.Next() != 0 {
		t.Error("switching sources kept the previous book")
	}
}

// TestSearchFieldOpensAnAddress types into the toolbar's search field, the
// one input of the empty window, and checks the address is opened.
func TestSearchFieldOpensAnAddress(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 820)

	tt.ClickAt(1080, 33)
	tt.Type("https://www.69shuba.com/book/90028/")
	tt.Key(0, ui.KeyEnter)

	if got := *a.session.Input(); got != "https://www.69shuba.com/book/90028/" {
		t.Fatalf("input = %q, want the typed address", got)
	}
	if a.session.Phase() != PhaseParsing {
		t.Fatalf("phase = %v, want parsing after Enter", a.session.Phase())
	}
}

// TestSearchFieldOnABookStartsEmpty checks a new address typed on a book
// screen replaces the search, rather than appending to the address that
// opened the book.
func TestSearchFieldOnABookStartsEmpty(t *testing.T) {
	a := testApp(t)
	*a.session.Input() = "https://www.69shuba.com/book/1/"
	a.session.catalog = &BookCatalog{
		Title:     "一本书",
		SourceURL: "https://www.69shuba.com/book/1/",
		Chapters:  []ChapterRef{{Title: "第一章", Order: 1}},
	}
	a.session.setPhase(PhaseCatalogReady, "", "")

	tt := ui.NewTester(a.view, 1280, 820)
	tt.ClickAt(1080, 33)
	tt.Type("https://www.69shuba.com/book/2/")
	tt.Key(0, ui.KeyEnter)

	if got := *a.session.Input(); got != "https://www.69shuba.com/book/2/" {
		t.Fatalf("input = %q, want the newly typed address", got)
	}
	if a.session.Phase() != PhaseParsing {
		t.Fatalf("phase = %v, want parsing after Enter", a.session.Phase())
	}
}

// TestPickingAResultRowOpensTheBook clicks a row of the results list, rather
// than calling the session directly.
func TestPickingAResultRowOpensTheBook(t *testing.T) {
	a := testApp(t)
	*a.session.Input() = "一本书"
	a.session.hits = []SearchHit{{
		Title: "一本书", Author: "作者", URL: "https://www.69shuba.com/book/1/",
	}}

	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("一本书"); err != nil {
		t.Fatal(err)
	}
	if a.session.Phase() != PhaseParsing {
		t.Fatalf("phase = %v, want parsing after the row click", a.session.Phase())
	}
	if got := *a.session.Input(); got != "https://www.69shuba.com/book/1/" {
		t.Fatalf("input = %q, want the book's address", got)
	}
}

// TestDeletingHistoryAsksFirst checks the history's destructive action opens
// a confirmation, and that the record survives until it is confirmed.
func TestDeletingHistoryAsksFirst(t *testing.T) {
	a := testApp(t)
	record := HistoryRecord{
		Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/",
		TotalChapters: 2, Chapters: []Chapter{{Title: "第一章", Order: 1}},
	}
	if _, err := a.history.Save(record); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("历史"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除记录"); err != nil {
		t.Fatal(err)
	}
	if !a.pendingDelete {
		t.Fatal("the delete did not ask for confirmation")
	}
	if a.history.Count() != 1 {
		t.Fatal("the record was deleted before it was confirmed")
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if a.history.Count() != 0 {
		t.Fatal("the record was not deleted after confirmation")
	}
}

// TestBrowserControlAppearsAfterAPageLoads checks the browser control is
// there once there is a page to show, and that it toggles the panel.
func TestBrowserControlAppearsAfterAPageLoads(t *testing.T) {
	a := testApp(t)
	if tt := ui.NewTester(a.view, 1280, 820); tt.HasText("浏览器") {
		t.Fatal("the browser control shows before a page has loaded")
	}

	a.panel.Open("https://www.69shuba.com/book/90028/")
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("浏览器") {
		t.Fatal("the browser control is missing after the page loaded")
	}

	a.showBrowser = true
	tt = ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("收起浏览器") {
		t.Fatal("the browser control does not offer to hide the panel")
	}
}

// TestChaptersDialogRemovesAChapter checks the edit mode hides the per-chapter
// action until it is entered.
func TestChaptersDialogRemovesAChapter(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{
		{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2},
	}}
	a.session.chapters = []Chapter{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}}
	a.session.setPhase(PhaseFinished, "完成", "")

	a.showChapters = true
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("已下载章节") || !tt.HasText("第一章") {
		t.Fatalf("chapter dialog missing; texts: %q", tt.Texts())
	}

	if err := tt.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("移除"); err != nil {
		t.Fatal(err)
	}
	if len(a.session.Chapters()) != 1 || a.session.Chapters()[0].Order != 2 {
		t.Fatalf("chapters after delete = %v, want the second only", a.session.Chapters())
	}
	if !tt.HasText("1 章") {
		t.Errorf("count not updated; texts: %q", tt.Texts())
	}
}

func TestExportDialogShowsSettings(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{{Title: "第一章", Order: 1}}}
	a.session.chapters = []Chapter{{Title: "第一章", Order: 1}}

	a.openExportDialog()
	tt := ui.NewTester(a.view, 1280, 820)
	if !a.showExport {
		t.Fatal("the export dialog did not open")
	}
	for _, want := range []string{"导出设置", "作者", "正文字体", "首页说明", "封面", "取消"} {
		if !tt.HasText(want) {
			t.Errorf("export dialog missing %q; texts: %q", want, tt.Texts())
		}
	}

	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.showExport {
		t.Error("the export dialog stayed open")
	}
}

func TestHistoryDialogListsSavedRecords(t *testing.T) {
	a := testApp(t)
	record := HistoryRecord{
		Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/",
		TotalChapters: 2, Chapters: []Chapter{{Title: "第一章", Order: 1}},
	}
	if _, err := a.history.Save(record); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("历史"); err != nil {
		t.Fatal(err)
	}
	if !a.showHistory {
		t.Fatal("the history dialog did not open")
	}
	for _, want := range []string{"下载历史", "一本书", "1 / 2", "部分 EPUB", "删除记录", "导出 EPUB"} {
		if !tt.HasText(want) {
			t.Errorf("history dialog missing %q; texts: %q", want, tt.Texts())
		}
	}
}

// TestPrimaryActionFollowsThePhase checks the one button is named for the next
// step, and never offers to export a book it already exported.
func TestPrimaryActionFollowsThePhase(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{Title: "一本书", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{
		{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2},
	}}

	a.session.setPhase(PhaseCatalogReady, "", "")
	if label, enabled, _ := a.topAction(); label != "开始下载" || !enabled {
		t.Errorf("catalog ready action = %q (%v), want 开始下载", label, enabled)
	}

	a.session.setPhase(PhaseDownloading, "", "")
	if label, _, _ := a.topAction(); label != "暂停下载" {
		t.Errorf("downloading action = %q, want 暂停下载", label)
	}

	a.session.chapters = []Chapter{{Title: "第一章", Order: 1}}
	a.session.setPhase(PhasePaused, "", "")
	if label, _, _ := a.topAction(); label != "继续下载" {
		t.Errorf("paused action = %q, want 继续下载", label)
	}

	a.session.setPhase(PhaseFinished, "", "")
	if label, _, _ := a.topAction(); label != "再导出一份" {
		t.Errorf("finished action = %q, want 再导出一份", label)
	}
}

// TestBrowserSearchKeepsThePanelOpen is the regression for a search that
// closed the panel before the user could solve the site's check.
func TestBrowserSearchKeepsThePanelOpen(t *testing.T) {
	a := testApp(t)

	a.session.searchThroughBrowser("武道")

	if !a.session.WantsPanel() {
		t.Error("the panel is not in front during a browser search")
	}
	if !a.panelVisible() {
		t.Error("the window would not draw the panel")
	}
	if a.session.Phase() != PhaseSearching {
		t.Errorf("phase = %v, want searching", a.session.Phase())
	}
}

// TestEmptySearchDoesNotOpenTheBrowser checks a site that simply has no such
// book is not treated as a blocked request.
func TestEmptySearchDoesNotOpenTheBrowser(t *testing.T) {
	a := testApp(t)

	a.session.finishSearch(nil)

	if a.session.WantsPanel() {
		t.Error("the browser opened for an empty result")
	}
	if a.session.Phase() != PhaseIdle {
		t.Errorf("phase = %v, want idle", a.session.Phase())
	}
	if !strings.Contains(a.session.Message(), "没有找到") {
		t.Errorf("message = %q, want it to say nothing was found", a.session.Message())
	}
}

// TestBrowserSearchGivesUpWhenThePageNeverLoads is the regression for a search
// that sat on "处理中…" forever behind a Cloudflare check: the page never
// fired its load event, so nothing ever read the results.
func TestBrowserSearchGivesUpWhenThePageNeverLoads(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the browser search deadline")
	}
	a := testApp(t)
	a.session.searchThroughBrowser("极道天魔")
	if a.session.Phase() != PhaseSearching {
		t.Fatalf("phase = %v, want searching", a.session.Phase())
	}

	// The page never settles. The deadline has to end the search anyway.
	waitFor(t, func() bool { return a.session.Phase() == PhaseFailed }, browserSearchTimeout+10*time.Second)

	if !strings.Contains(a.session.Message(), "没有加载完") {
		t.Errorf("message = %q, want it to say the page did not load", a.session.Message())
	}
	if a.session.Busy() {
		t.Error("the session is still busy after the deadline")
	}
}

// TestPauseTakesEffectImmediately is the regression for a pause that stopped
// the download but left the window saying "暂停下载": the phase only changed
// when the download goroutine noticed the cancellation.
func TestPauseTakesEffectImmediately(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{
		Title:     "一本书",
		SourceURL: "https://www.69shuba.com/book/1/",
		Chapters:  sampleRefs(100),
	}
	a.session.chapters = sampleChapters(20)
	a.session.next = 20
	a.session.setPhase(PhaseDownloading, "", "")

	a.session.pauseDownload()

	if a.session.Phase() != PhasePaused {
		t.Fatalf("phase = %v, want paused the moment it was pressed", a.session.Phase())
	}
	if a.session.Busy() {
		t.Error("a paused session still reports itself busy")
	}
	if label, _, _ := a.topAction(); label != "继续下载" {
		t.Errorf("action = %q, want 继续下载", label)
	}
	if !strings.Contains(a.session.Message(), "20 / 100") {
		t.Errorf("message = %q, want the kept count", a.session.Message())
	}
	// The record has to be on disk, so a quit right after a pause loses nothing.
	if a.history.Count() != 1 {
		t.Errorf("history records = %d, want the paused run saved", a.history.Count())
	}
	if got := a.history.List()[0].ProgressLabel(); got != "20 / 100" {
		t.Errorf("saved progress = %q, want 20 / 100", got)
	}
}

// TestPauseThenResumeContinuesFromTheKeptChapter checks the run picks up where
// the pause left it.
func TestPauseThenResumeContinuesFromTheKeptChapter(t *testing.T) {
	a := testApp(t)
	a.session.catalog = &BookCatalog{
		Title:     "一本书",
		SourceURL: "https://www.69shuba.com/book/1/",
		Chapters:  sampleRefs(100),
	}
	a.session.chapters = sampleChapters(20)
	a.session.next = 20
	a.session.setPhase(PhaseDownloading, "", "")

	a.session.pauseDownload()
	if !a.session.CanResume() {
		t.Fatal("a paused run cannot resume")
	}
	if a.session.Next() != 20 {
		t.Errorf("next = %d, want 20", a.session.Next())
	}
}
