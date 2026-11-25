package main

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func TestHistorySavesAndReloads(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	h := NewHistory(dir)

	id, err := h.Save(HistoryRecord{
		Title:         "极道天魔",
		Author:        "滚开",
		SourceURL:     "https://www.69shuba.com/book/90028/",
		TotalChapters: 567,
		Chapters:      []Chapter{{Title: "第一章 夜路", Order: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("a saved record has no ID")
	}

	list := h.List()
	if len(list) != 1 || list[0].ProgressLabel() != "1 / 567" {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Complete() {
		t.Error("a partial record reports itself complete")
	}

	h.MarkExported(id, "极道天魔-partial-1-of-567.epub")
	reloaded := NewHistory(dir)
	record, ok := reloaded.Get(id)
	if !ok {
		t.Fatal("the record did not survive a reload")
	}
	if record.ExportName != "极道天魔-partial-1-of-567.epub" {
		t.Errorf("export name = %q", record.ExportName)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		t.Error("the record has no timestamps")
	}

	if err := reloaded.Delete(id); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.List()) != 0 {
		t.Error("the record survived deletion")
	}
}

// TestHistoryDedupesBySourceURL checks that downloading the same book twice
// leaves one entry, not two.
func TestHistoryDedupesBySourceURL(t *testing.T) {
	h := NewHistory(filepath.Join(t.TempDir(), "history"))
	const source = "https://www.69shuba.com/book/90028/"

	first, err := h.Save(HistoryRecord{
		Title: "极道天魔", SourceURL: source, TotalChapters: 10,
		Chapters: []Chapter{{Title: "第一章", Order: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Save(HistoryRecord{
		Title: "极道天魔", SourceURL: source, TotalChapters: 10,
		Chapters: []Chapter{{Title: "第一章", Order: 1}, {Title: "第二章", Order: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Errorf("second save made a new record: %q then %q", first, second)
	}
	list := h.List()
	if len(list) != 1 {
		t.Fatalf("%d records, want the book stored once", len(list))
	}
	if list[0].Downloaded != 2 {
		t.Errorf("downloaded = %d, want the newer save", list[0].Downloaded)
	}
}

// TestHistoryMigratesTheOldFile checks the single-file history is read once
// and kept as a .bak.
func TestHistoryMigratesTheOldFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	legacy := filepath.Join(dir, "history.json")
	if err := writeFileAtomic(legacy, []byte(`[{"id":"old-1","title":"旧书","sourceURL":"https://sto9.com/book/1/index.html","totalChapters":3,"chapters":[{"title":"第一章","order":1}],"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z"}]`)); err != nil {
		t.Fatal(err)
	}

	h := NewHistory(dir)
	list := h.List()
	if len(list) != 1 || list[0].Title != "旧书" || list[0].ProgressLabel() != "1 / 3" {
		t.Fatalf("migrated list = %+v", list)
	}
	record, ok := h.Get("old-1")
	if !ok || len(record.Chapters) != 1 {
		t.Fatalf("migrated record = %+v", record)
	}
	if _, err := os.Stat(legacy + ".bak"); err != nil {
		t.Errorf("the old file was not kept as .bak: %v", err)
	}
	if _, err := os.Stat(legacy); err == nil {
		t.Error("the old file is still in place")
	}
}

func TestCrawlerReadsCatalogAndChapter(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/book/90028/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><head>
			<meta property="og:novel:book_name" content="极道天魔"/>
			<meta property="og:novel:author" content="滚开"/>
			</head><body><ul>
			<li><a href="/txt/90028/1">第一章 夜路</a></li>
			<li><a href="/txt/90028/2">第二章 敲门</a></li>
			</ul></body></html>`)
	})
	mux.HandleFunc("/txt/90028/1", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><body><h1>第一章 夜路</h1>
			<div id="chaptercontent">第一段内容。<br/><br/>第二段内容。<br/></div>
			<div id="bottomlink"></div></body></html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base = server.URL

	crawler := NewCrawler(SourceFor(SourceShuba69), "JustNovel test", base+"/book/90028/", nil, nil)
	ctx := context.Background()

	catalog, err := crawler.FetchCatalog(ctx, base+"/book/90028/", "")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Title != "极道天魔" || catalog.Author != "滚开" || len(catalog.Chapters) != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}
	if got := catalog.Chapters[0].URL; got != base+"/txt/90028/1" {
		t.Errorf("first chapter URL = %q", got)
	}

	chapter, transport, err := crawler.FetchChapter(ctx, catalog.Chapters[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if transport != TransportSession {
		t.Errorf("transport = %v, want the session", transport)
	}
	if chapter.PlainText != "第一段内容。\n\n第二段内容。" {
		t.Errorf("chapter text = %q", chapter.PlainText)
	}
}

func TestCrawlerFallsBackToTheBrowser(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/txt/15/1.html", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base = server.URL

	chapterHTML := `<html><body>
		<h1>第3章 前世今生皆为我</h1>
		<div class="toplink"><a href="./">返回目录</a></div>
		<div id="content">单雄虽然人品差，可在武毅学堂的一年不是白混的。<br/><br/>助教们看到后，就准备让人上去抬人了。<br/><br/></div>
		<div class="bottomlink"></div>
		</body></html>`

	var loaded string
	browser := func(ctx context.Context, page string) (string, error) {
		loaded = page
		return chapterHTML, nil
	}

	crawler := NewCrawler(SourceFor(SourcePiaotian), "", base, nil, browser)
	chapter, transport, err := crawler.FetchChapter(context.Background(), ChapterRef{
		Title: "第3章 前世今生皆为我",
		URL:   base + "/txt/15/1.html",
		Order: 3,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if transport != TransportBrowser {
		t.Errorf("transport = %v, want the browser", transport)
	}
	if loaded != base+"/txt/15/1.html" {
		t.Errorf("browser loaded %q", loaded)
	}
	if chapter.Order != 3 || !strings.Contains(chapter.PlainText, "单雄虽然人品差") {
		t.Errorf("chapter = %+v", chapter)
	}
}

func TestCrawlerMergesTheCompleteCatalog(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/book/13564/index.html", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><head><meta property="og:novel:book_name" content="腐朽世界"/></head><body>
			<ul>
			<li data-num="1"><a href="https://sto9.com/txt/13564/1.html">001 覺醒</a>
			<li data-num="2"><a href="https://sto9.com/txt/13564/2.html">002 決斷</a>
			</ul></body></html>`)
	})
	mux.HandleFunc("/ajax_novels/chapterlist/13564.html", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<ul>
			<li data-num="1"><a href="https://sto9.com/txt/13564/1.html">001 覺醒</a>
			<li data-num="2"><a href="https://sto9.com/txt/13564/2.html">002 決斷</a>
			<li data-num="3"><a href="https://sto9.com/txt/13564/3.html">003 搬家</a>
			</ul>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	base := server.URL + "/book/13564/index.html"
	crawler := NewCrawler(SourceFor(SourceSto9), "", base, nil, nil)
	catalog, err := crawler.FetchCatalog(context.Background(), base, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Chapters) != 3 {
		t.Fatalf("%d chapters, want the 3 of the complete list", len(catalog.Chapters))
	}
	if catalog.Chapters[2].Order != 3 || !strings.Contains(catalog.Chapters[2].Title, "搬家") {
		t.Errorf("last chapter = %+v", catalog.Chapters[2])
	}
}

func TestShuba69ForbiddenFallsBackImmediately(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/txt/90028/1", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	chapterHTML := `<html><body><h1>第一章 夜路</h1>
		<div id="chaptercontent">第一段内容足够长，可以通过正文校验。<br/><br/>第二段内容也足够长，同样可以通过校验。<br/></div>
		<div id="bottomlink"></div></body></html>`
	browser := func(ctx context.Context, page string) (string, error) { return chapterHTML, nil }

	crawler := NewCrawler(SourceFor(SourceShuba69), "", server.URL+"/book/90028/", nil, browser)
	started := time.Now()
	_, transport, err := crawler.FetchChapter(context.Background(), ChapterRef{
		Title: "第一章 夜路",
		URL:   server.URL + "/txt/90028/1",
		Order: 1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if transport != TransportBrowser {
		t.Errorf("transport = %v, want the browser", transport)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("a 403 took %s, want an immediate browser fallback", elapsed)
	}
}

func TestCrawlerRetriesRateLimits(t *testing.T) {
	var hits atomic.Int32
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/txt/1/1.html", func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `<html><body>
			<h1>第1章 开始</h1>
			<div class="toplink"><a href="./">返回目录</a></div>
			<div id="content">这是第一段正文内容，长度足够通过校验。<br/><br/>这是第二段正文内容，同样足够长。<br/><br/></div>
			<div class="bottomlink"></div></body></html>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base = server.URL

	source := SourceFor(SourcePiaotian)
	source.RateLimitDelay = 10 * time.Millisecond

	crawler := NewCrawler(source, "", base, nil, nil)
	chapter, _, err := crawler.FetchChapter(context.Background(), ChapterRef{
		Title: "第1章 开始",
		URL:   base + "/txt/1/1.html",
		Order: 1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Errorf("server saw %d requests, want 2", hits.Load())
	}
	if !strings.Contains(chapter.PlainText, "这是第二段正文内容") {
		t.Errorf("chapter = %+v", chapter)
	}
}

func TestChallengePagesNeedTheBrowser(t *testing.T) {
	if !looksLikeChallengePage(`<html><title>Just a moment...</title><script>window.cf_chl_opt={}</script></html>`) {
		t.Error("a Cloudflare challenge page is not recognized")
	}
	if looksLikeChallengePage(`<html><body>正文</body></html>`) {
		t.Error("a normal page is taken for a challenge")
	}
}

func TestDecodePageFallsBackToGB18030(t *testing.T) {
	want := "极道天魔 第一章"
	gbk, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if got := decodePage(gbk); got != want {
		t.Errorf("decoded %q, want %q", got, want)
	}

	utf16Text := utf16.Encode([]rune(want))
	raw := make([]byte, 2+2*len(utf16Text))
	raw[0], raw[1] = 0xFF, 0xFE
	for i, r := range utf16Text {
		binary.LittleEndian.PutUint16(raw[2+i*2:], r)
	}
	if got := decodePage(raw); got != want {
		t.Errorf("UTF-16 decoded %q, want %q", got, want)
	}
}
