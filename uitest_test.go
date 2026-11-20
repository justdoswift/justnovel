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

var shootStates = []shootState{
	{"01-empty", func(a *app) {}},
	{"02-results", func(a *app) {
		a.query = "武道"
		a.hits = []SearchHit{
			{Title: "苟在武道世界成圣", Author: "南山南", URL: "https://www.69shuba.com/book/90028/"},
			{Title: "武道宗师", Author: "爱潜水的乌贼", URL: "https://www.69shuba.com/book/1/"},
			{Title: "武道独尊", Author: "纯情犀利哥", URL: "https://www.69shuba.com/book/2/"},
		}
	}},
	{"03-catalog", func(a *app) {
		a.urlText = "https://www.69shuba.com/book/90028/"
		a.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.job = JobState{Phase: PhaseCatalogReady, Message: "目录已就绪，共 120 章。确认后开始下载。"}
	}},
	{"03-downloading", func(a *app) {
		a.urlText = "https://www.69shuba.com/book/90028/"
		a.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.chapters = sampleChapters(47)
		a.next = 47
		a.running = true
		a.startedAt = time.Now().Add(-4 * time.Minute)
		a.startCount = 0
		seedLog(a, 47)
		a.job = JobState{Phase: PhaseDownloading, Snap: &Progress{
			Completed: 47, Total: 120,
			CurrentTitle: "第48章 断剑", CurrentIndex: 48,
			LastTitle: "第47章 夜行", LastIndex: 47, HasLast: true,
		}}
	}},
	{"04-finished", func(a *app) {
		a.urlText = "https://www.69shuba.com/book/90028/"
		a.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.chapters = sampleChapters(120)
		a.next = 120
		a.lastSaved = "/Users/gepeng/Downloads/苟在武道世界成圣.epub"
		seedLog(a, 120)
		a.job = JobState{Phase: PhaseFinished, Message: "EPUB 已导出：苟在武道世界成圣.epub"}
	}},
	{"05-paused", func(a *app) {
		a.urlText = "https://www.69shuba.com/book/90028/"
		a.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.chapters = sampleChapters(47)
		a.next = 47
		a.startedAt = time.Now().Add(-4 * time.Minute)
		seedLog(a, 47)
		a.logf(LogWarn, "已暂停", "保留 47 / 120 章")
		a.job = JobState{Phase: PhasePaused, Snap: &Progress{
			Completed: 47, Total: 120,
			CurrentTitle: "第48章 断剑", CurrentIndex: 48,
			LastTitle: "第47章 夜行", LastIndex: 47, HasLast: true,
		}}
	}},
	{"06-failed", func(a *app) {
		a.urlText = "https://www.69shuba.com/book/90028/"
		a.catalog = &BookCatalog{
			Title:     "苟在武道世界成圣",
			Author:    "南山南",
			SourceURL: "https://www.69shuba.com/book/90028/",
			Chapters:  sampleRefs(120),
		}
		a.chapters = sampleChapters(47)
		a.next = 47
		a.startedAt = time.Now().Add(-4 * time.Minute)
		seedLog(a, 47)
		a.logf(LogError, "下载中断", "第 48 章请求被站点拦截，重试 3 次后仍失败。")
		a.job = JobState{Phase: PhaseFailed, Message: "第 48 章请求被站点拦截，重试 3 次后仍失败。"}
	}},
}

// seedLog fills the log with the kind of lines a real run leaves behind, so
// the screenshots show the surface as it actually reads.
func seedLog(a *app, chapters int) {
	a.logReset()
	base := time.Date(2026, 11, 20, 13, 40, 0, 0, time.Local)
	add := func(offset int, kind LogKind, text, detail string) {
		a.logSeq++
		a.log = append(a.log, LogLine{Seq: a.logSeq, At: base.Add(time.Duration(offset) * time.Second), Kind: kind, Text: text, Detail: detail})
	}
	addTook := func(offset int, kind LogKind, text, detail string, took time.Duration) {
		a.logSeq++
		a.log = append(a.log, LogLine{Seq: a.logSeq, At: base.Add(time.Duration(offset) * time.Second), Kind: kind, Text: text, Detail: detail, Took: took})
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
