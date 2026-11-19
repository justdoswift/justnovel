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

	for _, want := range []string{"justnovel", "准备开始", "BROWSER", "69 书吧"} {
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
	tt := ui.NewTester(a.view, 1280, 820)

	a.urlText = "https://example.com/not-a-catalog"
	tt.Frame()
	if !tt.HasText(a.source.InvalidText) {
		t.Errorf("invalid URL error missing; texts: %q", tt.Texts())
	}
	if a.canParse() {
		t.Error("an invalid URL can be parsed")
	}
}

func TestOpenSourceWaitsForSession(t *testing.T) {
	a := testApp(t)
	a.urlText = a.source.Placeholder
	tt := ui.NewTester(a.view, 1280, 820)

	if err := tt.Click("打开目录"); err != nil {
		t.Fatal(err)
	}
	if a.job.Phase != PhaseSessionRequired {
		t.Fatalf("phase = %v, want session required", a.job.Phase)
	}
	if !tt.HasText("等待站点会话") {
		t.Errorf("status headline missing; texts: %q", tt.Texts())
	}
	if !a.showBrowser {
		t.Error("the browser panel is hidden after opening a catalog")
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
	if a.urlText != "" {
		t.Errorf("URL text = %q, want an empty target after switching sources", a.urlText)
	}
}

func TestChaptersDialogRemovesAChapter(t *testing.T) {
	a := testApp(t)
	a.catalog = &BookCatalog{Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{
		{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2},
	}}
	a.chapters = []Chapter{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}}
	a.job = JobState{Phase: PhaseFinished, Message: "完成"}

	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("查看已下载章节"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已下载章节") || !tt.HasText("第一章") {
		t.Fatalf("chapter dialog missing; texts: %q", tt.Texts())
	}

	if err := tt.Click("移除"); err != nil {
		t.Fatal(err)
	}
	if len(a.chapters) != 1 || a.chapters[0].Order != 2 {
		t.Fatalf("chapters after delete = %v, want the second only", a.chapters)
	}
	if !tt.HasText("1 CHAPTERS") {
		t.Errorf("count not updated; texts: %q", tt.Texts())
	}
}

func TestExportDialogShowsSettings(t *testing.T) {
	a := testApp(t)
	a.catalog = &BookCatalog{Title: "一本书", Author: "作者", SourceURL: "https://www.69shuba.com/book/1/", Chapters: []ChapterRef{{Title: "第一章", Order: 1}}}
	a.chapters = []Chapter{{Title: "第一章", Order: 1}}

	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("导出当前内容"); err != nil {
		t.Fatal(err)
	}
	if !a.showExport {
		t.Fatal("the export dialog did not open")
	}
	for _, want := range []string{"导出设置", "AUTHOR", "FONT", "FRONT PAGE", "COVER", "取消"} {
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
	if err := tt.Click("打开下载历史"); err != nil {
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
