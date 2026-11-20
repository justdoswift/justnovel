package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testApp(t *testing.T) *app {
	t.Helper()
	a := newApp()
	a.history = NewHistory(filepath.Join(t.TempDir(), "history.json"))
	a.session = newBrowserSession(nil, nil, a.source)
	return a
}

func TestWindowShowsStatusAndSource(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 820)

	for _, want := range []string{"justnovel", "搜一本书", "69 书吧"} {
		if !tt.HasText(want) {
			t.Errorf("window does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if a.urlText != "" {
		t.Errorf("source field = %q, want an empty target", a.urlText)
	}
}

func TestInvalidURLShowsError(t *testing.T) {
	a := testApp(t)

	a.urlText = "https://example.com/not-a-catalog"
	if a.canParse() {
		t.Error("an invalid URL can be parsed")
	}
	a.openSource()
	if a.job.Phase != PhaseFailed {
		t.Fatalf("phase = %v, want failed for an invalid address", a.job.Phase)
	}
	if a.job.Message != a.source.InvalidText {
		t.Errorf("message = %q, want %q", a.job.Message, a.source.InvalidText)
	}
}

// Opening a catalog reads it without asking for a second press, and the
// browser stays out of the way until a site demands a human.
func TestOpenSourceReadsTheCatalogInOneStep(t *testing.T) {
	a := testApp(t)
	a.urlText = "https://www.69shuba.com/book/90028/"

	a.openSource()

	if a.job.Phase != PhaseParsing {
		t.Fatalf("phase = %v, want parsing right after opening", a.job.Phase)
	}
	if a.showBrowser {
		t.Error("the browser panel opened although the site asked for nothing")
	}
}

func TestReopeningTheSameCatalogKeepsProgress(t *testing.T) {
	a := testApp(t)
	a.urlText = "https://www.69shuba.com/book/90028/"
	a.catalog = &BookCatalog{
		Title:     "一本书",
		SourceURL: "https://www.69shuba.com/book/90028/",
		Chapters:  []ChapterRef{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}},
	}
	a.chapters = []Chapter{{Title: "第一章", Order: 1}}
	a.next = 1

	a.openSource()

	if len(a.chapters) != 1 || a.catalog == nil || a.catalog.Title != "一本书" {
		t.Fatalf("reopening the catalog lost the download: chapters=%d catalog=%v", len(a.chapters), a.catalog)
	}
	if a.job.Phase == PhaseSessionRequired {
		t.Error("reopening the catalog reset the job to session required")
	}
}

func TestOpenSourceNormalizesAChapterPage(t *testing.T) {
	a := testApp(t)
	a.source = SourceFor(SourcePiaotian)
	a.session.Configure(a.source)
	a.urlText = "https://www.piaotia.com/html/15/15709/11428790.html"

	a.openSource()

	if a.urlText != "https://www.piaotia.com/html/15/15709/" {
		t.Errorf("URL = %q, want the catalog page", a.urlText)
	}
	if a.job.Phase != PhaseParsing {
		t.Errorf("phase = %v, want parsing right after opening", a.job.Phase)
	}
}

func TestStopDownloadKeepsTheHistoryRecord(t *testing.T) {
	a := testApp(t)
	a.catalog = &BookCatalog{
		Title:     "一本书",
		Author:    "作者",
		SourceURL: "https://www.69shuba.com/book/1/",
		Chapters:  []ChapterRef{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}},
	}
	a.chapters = []Chapter{{Title: "第一章", Order: 1, PlainText: "正文"}}
	a.running = true
	a.job = JobState{Phase: PhaseDownloading}

	a.stopDownload()

	if a.history.Count() != 1 {
		t.Fatalf("history records = %d, want the stopped download saved", a.history.Count())
	}
	if len(a.chapters) != 0 || a.job.Phase != PhaseStopped {
		t.Fatalf("stop left chapters=%d phase=%v", len(a.chapters), a.job.Phase)
	}
}

func TestSourceSwitchResetsTheBook(t *testing.T) {
	a := testApp(t)
	a.catalog = &BookCatalog{Title: "一本书", Chapters: []ChapterRef{{Title: "第一章", Order: 1}}}
	a.chapters = []Chapter{{Title: "第一章", Order: 1}}
	a.next = 1

	a.selectSource(SourcePiaotian)

	if a.source.Kind != SourcePiaotian {
		t.Errorf("source = %v", a.source.Kind)
	}
	if a.catalog != nil || len(a.chapters) != 0 || a.next != 0 {
		t.Error("switching sources kept the previous book")
	}
	// The field is filled with the new site's example rather than emptied, so
	// switching never leaves the user with nothing to press Enter on.
	if a.urlText != a.source.Example {
		t.Errorf("URL text = %q, want the new site's example %q", a.urlText, a.source.Example)
	}
	if !a.canParse() {
		t.Error("the address after switching sources cannot be opened")
	}
}

func TestChaptersDialogRemovesAChapter(t *testing.T) {
	a := testApp(t)
	a.catalog = &BookCatalog{Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{
		{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2},
	}}
	a.chapters = []Chapter{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}}
	a.job = JobState{Phase: PhaseFinished, Message: "完成"}

	a.showChapters = true
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("已下载章节") || !tt.HasText("第一章") {
		t.Fatalf("chapter dialog missing; texts: %q", tt.Texts())
	}

	if err := tt.Click("移除"); err != nil {
		t.Fatal(err)
	}
	if len(a.chapters) != 1 || a.chapters[0].Order != 2 {
		t.Fatalf("chapters after delete = %v, want the second only", a.chapters)
	}
	if !tt.HasText("1 章") {
		t.Errorf("count not updated; texts: %q", tt.Texts())
	}
}

func TestExportDialogShowsSettings(t *testing.T) {
	a := testApp(t)
	a.catalog = &BookCatalog{Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{{Title: "第一章", Order: 1}}}
	a.chapters = []Chapter{{Title: "第一章", Order: 1}}

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

func TestPastedURLSelectsItsSource(t *testing.T) {
	a := testApp(t)

	a.urlText = "https://www.piaotia.com/html/15/15709/"
	a.detectSource()
	if a.source.Kind != SourcePiaotian || !a.canParse() {
		t.Fatalf("piaotian URL left source %v, canParse %v", a.source.Kind, a.canParse())
	}
	if a.urlText != "https://www.piaotia.com/html/15/15709/" {
		t.Errorf("address changed to %q", a.urlText)
	}

	a.urlText = "https://sto9.com/txt/13564/8169182.html"
	a.detectSource()
	if a.source.Kind != SourceSto9 || !a.canParse() {
		t.Fatalf("sto9 URL left source %v, canParse %v", a.source.Kind, a.canParse())
	}

	a.urlText = "https://www.69shuba.com/book/90028.htm"
	a.detectSource()
	if a.source.Kind != SourceShuba69 || !a.canParse() {
		t.Fatalf("69shuba URL left source %v, canParse %v", a.source.Kind, a.canParse())
	}
}

// Switching the site fills the field with that site's address, so the user
// never has to paste again after picking a different source.
func TestSwitchingSourceFillsTheAddress(t *testing.T) {
	a := testApp(t)
	a.urlText = "https://www.69shuba.com/book/90028/"

	a.selectSource(SourcePiaotian)
	if a.urlText != a.source.Example {
		t.Fatalf("address = %q, want the piaotian example %q", a.urlText, a.source.Example)
	}
	if !a.canParse() {
		t.Error("the filled address cannot be opened")
	}

	a.selectSource(SourceSto9)
	if a.urlText != a.source.Example {
		t.Fatalf("address = %q, want the sto9 example %q", a.urlText, a.source.Example)
	}
	if !a.canParse() {
		t.Error("the filled address cannot be opened")
	}
}

// An address that already belongs to the new site is kept as it is.
func TestSwitchingSourceKeepsAMatchingAddress(t *testing.T) {
	a := testApp(t)
	a.source = SourceFor(SourcePiaotian)
	a.session.Configure(a.source)
	a.urlText = "https://www.piaotia.com/html/15/15709/"

	a.selectSource(SourcePiaotian)
	if a.urlText != "https://www.piaotia.com/html/15/15709/" {
		t.Errorf("address = %q, want it left alone", a.urlText)
	}
}
