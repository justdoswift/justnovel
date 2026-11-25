package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestShootStates renders the window in each state it can be in and writes
// the frames to .oil-ui/shots, so a redesign can be looked at without
// launching the app.
func TestShootStates(t *testing.T) {
	if os.Getenv("SHOOT") == "" {
		t.Skip("set SHOOT=1 to render the frames")
	}
	out := filepath.Join(".oil-ui", "shots")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, state := range shootStates {
		state := state
		t.Run(state.name, func(t *testing.T) {
			a := testApp(t)
			state.build(a)
			tt := ui.NewTester(a.view, 1280, 820)
			tt.Frame()
			img := tt.Image()
			file, err := os.Create(filepath.Join(out, state.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, img); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type shootState struct {
	name  string
	build func(*app)
}

// TestShootMoreMenu renders the status bar's more menu open, which the
// per-state shots cannot show without an interaction.
func TestShootMoreMenu(t *testing.T) {
	if os.Getenv("SHOOT") == "" {
		t.Skip("set SHOOT=1 to render the frames")
	}
	out := filepath.Join(".oil-ui", "shots")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	a := testApp(t)
	for _, state := range shootStates {
		if state.name == "05-paused" {
			state.build(a)
			break
		}
	}
	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()
	// The more button ends the status bar: 20 from the right edge, 28 wide.
	tt.ClickAt(1280-20-14, 820-26)
	tt.Frame()

	file, err := os.Create(filepath.Join(out, "13-more-menu.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

var shootStates = []shootState{
	{"01-empty", func(a *app) {}},
	{"02-results", func(a *app) {
		*a.session.Input() = "武道"
		a.session.hits = []SearchHit{
			{Title: "苟在武道世界成圣", Author: "南山南", URL: "https://www.69shuba.com/book/90028/"},
			{Title: "武道宗师", Author: "爱潜水的乌贼", URL: "https://www.69shuba.com/book/1/"},
			{Title: "武道独尊", Author: "纯情犀利哥", URL: "https://www.69shuba.com/book/2/"},
		}
	}},
	{"03-catalog", func(a *app) {
		*a.session.Input() = "https://www.69shuba.com/book/90028/"
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.setPhase(PhaseCatalogReady, "目录已就绪，共 120 章。", "")
	}},
	{"03-downloading", func(a *app) {
		*a.session.Input() = "https://www.69shuba.com/book/90028/"
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(47)
		a.session.next = 47
		a.session.startedAt = time.Now().Add(-4 * time.Minute)
		a.session.startCount = 0
		seedLog(a, 47)
		a.session.setPhase(PhaseDownloading, "", "")
		a.session.progress = &Progress{
			Completed: 47, Total: 120,
			CurrentTitle: "第48章 断剑", CurrentIndex: 48,
			LastTitle: "第47章 夜行", LastIndex: 47, HasLast: true,
		}
	}},
	{"04-finished", func(a *app) {
		*a.session.Input() = "https://www.69shuba.com/book/90028/"
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(120)
		a.session.next = 120
		a.session.lastSaved = "/Users/gepeng/Downloads/苟在武道世界成圣.epub"
		seedLog(a, 120)
		a.session.setPhase(PhaseFinished, "EPUB 已导出：苟在武道世界成圣.epub", "")
	}},
	{"05-paused", func(a *app) {
		*a.session.Input() = "https://www.69shuba.com/book/90028/"
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(47)
		a.session.next = 47
		a.session.startedAt = time.Now().Add(-4 * time.Minute)
		seedLog(a, 47)
		a.session.logf(LogWarn, "已暂停", "保留 47 / 120 章")
		a.session.setPhase(PhasePaused, "已暂停，保留 47 / 120 章。", "")
		a.session.progress = &Progress{
			Completed: 47, Total: 120,
			CurrentTitle: "第48章 断剑", CurrentIndex: 48,
			LastTitle: "第47章 夜行", LastIndex: 47, HasLast: true,
		}
	}},
	{"07-export-dialog", func(a *app) {
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(120)
		a.session.next = 120
		a.draft = ExportSettings{
			Title:      "苟在武道世界成圣",
			Author:     "南山南",
			BodyFont:   FontSongti,
			FrontText:  "献给每一位读到这里的人。",
			ShowSource: true,
		}
		a.exportFont = FontSongti.DisplayName()
		a.session.setPhase(PhaseFinished, "EPUB 已导出：苟在武道世界成圣.epub", "")
		a.showExport = true
	}},
	{"08-history-dialog", func(a *app) {
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		full := HistoryRecord{
			ID:            "r1",
			Title:         "苟在武道世界成圣",
			Author:        "南山南",
			SourceURL:     "https://www.69shuba.com/book/90028/",
			TotalChapters: 120,
			Chapters:      sampleChapters(120),
			CreatedAt:     time.Date(2026, 11, 2, 21, 0, 0, 0, time.Local),
			UpdatedAt:     time.Date(2026, 11, 20, 22, 30, 0, 0, time.Local),
			Settings:      ExportSettings{Title: "苟在武道世界成圣", BodyFont: FontSongti},
		}
		partial := HistoryRecord{
			ID:            "r2",
			Title:         "武道宗师",
			Author:        "爱潜水的乌贼",
			SourceURL:     "https://www.69shuba.com/book/1/",
			TotalChapters: 800,
			Chapters:      sampleChapters(213),
			CreatedAt:     time.Date(2026, 10, 18, 9, 0, 0, 0, time.Local),
			UpdatedAt:     time.Date(2026, 11, 8, 23, 15, 0, 0, time.Local),
			Settings:      ExportSettings{Title: "武道宗师", BodyFont: FontSystem},
		}
		for _, record := range []HistoryRecord{full, partial} {
			if _, err := a.history.Save(record); err != nil {
				panic(err)
			}
		}
		a.historyList = a.history.List()
		a.selectedID = "r1"
		a.showHistory = true
	}},
	{"09-chapters-dialog", func(a *app) {
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(47)
		a.session.next = 47
		a.session.startedAt = time.Now().Add(-4 * time.Minute)
		seedLog(a, 47)
		a.session.setPhase(PhasePaused, "已暂停，保留 47 / 120 章。", "")
		a.session.progress = &Progress{
			Completed: 47, Total: 120,
			CurrentTitle: "第48章 断剑", CurrentIndex: 48,
			LastTitle: "第47章 夜行", LastIndex: 47, HasLast: true,
		}
		a.showChapters = true
	}},
	{"10-chapters-editing", func(a *app) {
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(47)
		a.session.next = 47
		a.session.startedAt = time.Now().Add(-4 * time.Minute)
		seedLog(a, 47)
		a.session.setPhase(PhasePaused, "已暂停，保留 47 / 120 章。", "")
		a.session.progress = &Progress{
			Completed: 47, Total: 120,
			CurrentTitle: "第48章 断剑", CurrentIndex: 48,
			LastTitle: "第47章 夜行", LastIndex: 47, HasLast: true,
		}
		a.showChapters = true
		a.editChapters = true
	}},
	{"11-stop-alert", func(a *app) {
		a.pendingStop = true
	}},
	{"12-delete-alert", func(a *app) {
		a.pendingDelete = true
	}},
	{"06-failed", func(a *app) {
		*a.session.Input() = "https://www.69shuba.com/book/90028/"
		a.session.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.session.chapters = sampleChapters(47)
		a.session.next = 47
		a.session.startedAt = time.Now().Add(-4 * time.Minute)
		seedLog(a, 47)
		a.session.logf(LogError, "下载中断", "第 48 章请求被站点拦截，重试 3 次后仍失败。")
		a.session.setPhase(PhaseFailed, "下载中断。", "第 48 章请求被站点拦截，重试 3 次后仍失败。")
	}},
}

// seedLog fills the log with the kind of lines a real run leaves behind, so
// the screenshots show the surface as it actually reads.
func seedLog(a *app, chapters int) {
	a.session.logReset()
	base := time.Date(2026, 11, 20, 13, 40, 0, 0, time.Local)
	add := func(offset int, kind LogKind, text, detail string) {
		a.session.logSeq++
		a.session.log = append(a.session.log, LogLine{Seq: a.session.logSeq, At: base.Add(time.Duration(offset) * time.Second), Kind: kind, Text: text, Detail: detail})
	}
	addTook := func(offset int, kind LogKind, text, detail string, took time.Duration) {
		a.session.logSeq++
		a.session.log = append(a.session.log, LogLine{Seq: a.session.logSeq, At: base.Add(time.Duration(offset) * time.Second), Kind: kind, Text: text, Detail: detail, Took: took})
	}
	add(0, LogInfo, "开始下载", "苟在武道世界成圣 · 共 120 章")
	refs := sampleRefs(chapters)
	for i, ref := range refs {
		addTook(2+i*3, LogChapter, fmt.Sprintf("%03d  %s", ref.Order, ref.Title), "", 4*time.Second+time.Duration(i%3)*time.Second)
		if i == 12 {
			add(2+i*3+1, LogFallback, "切换到浏览器抓取", "013  第13章 相聚")
		}
	}
	a.logState.FollowEnd = true
}

func sampleRefs(n int) []ChapterRef {
	titles := []string{"乱世", "老宅", "表姐", "相聚", "拜师", "麻烦", "袭杀", "断剑", "夜行", "归途"}
	refs := make([]ChapterRef, n)
	for i := range refs {
		refs[i] = ChapterRef{
			Title: "第" + itoa(i+1) + "章 " + titles[i%len(titles)],
			Order: i + 1,
		}
	}
	return refs
}

func sampleChapters(n int) []Chapter {
	refs := sampleRefs(n)
	out := make([]Chapter, n)
	for i, ref := range refs {
		out[i] = Chapter{Title: ref.Title, Order: ref.Order, PlainText: "正文"}
	}
	return out
}
