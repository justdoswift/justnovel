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

	for _, want := range []string{"JustNovel", "准备开始", "同步目录、下载章节、导出 EPUB", "浏览器会话", "69 书吧"} {
		if !tt.HasText(want) {
			t.Errorf("window does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if a.urlText != a.source.Placeholder {
		t.Errorf("source field = %q, want the placeholder %q", a.urlText, a.source.Placeholder)
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
	if a.urlText != SourceFor(SourcePiaotian).Placeholder {
		t.Errorf("URL text = %q", a.urlText)
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

	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if len(a.chapters) != 1 || a.chapters[0].Order != 2 {
		t.Fatalf("chapters after delete = %v, want the second only", a.chapters)
	}
	if !tt.HasText("共 1 章") {
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
	for _, want := range []string{"导出设置", "作者信息", "正文字体", "首页内容", "封面", "取消"} {
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
	if err := tt.Click("导出历史"); err != nil {
		t.Fatal(err)
	}
	if !a.showHistory {
		t.Fatal("the history dialog did not open")
	}
	for _, want := range []string{"下载历史", "一本书", "1 / 2", "部分 EPUB", "删除记录", "导出文件"} {
		if !tt.HasText(want) {
			t.Errorf("history dialog missing %q; texts: %q", want, tt.Texts())
		}
	}
}
