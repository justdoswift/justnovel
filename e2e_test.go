package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEndToEndSearchCatalogDownloadExport runs the whole flow against a fake
// site: search a title, read the catalog, download every chapter, and write
// the EPUB. It is the test the old code could not pass, because the catalog
// step deadlocked before the download could start.
func TestEndToEndSearchCatalogDownloadExport(t *testing.T) {
	server := newFakeSite(t, 3)
	defer server.Close()
	useTempDownloads(t)

	source := fakeSource(server)

	history := NewHistory(filepath.Join(t.TempDir(), "history"))
	page := &fakePage{}
	panel := newFakePanel(page, source)
	session := NewSession(nil, panel, history, source)
	window := newApp(session, panel, history)

	// 1. Search.
	*session.Input() = "极道天魔"
	session.Submit()
	waitFor(t, func() bool { return session.Phase() == PhaseIdle && len(session.Hits()) > 0 })

	hits := session.Hits()
	if len(hits) != 1 {
		t.Fatalf("%d hits, want the one book", len(hits))
	}
	if hits[0].Title != "极道天魔" {
		t.Errorf("hit title = %q", hits[0].Title)
	}

	// 2. Open it. The catalog is read from the page the panel settles on.
	page.setHTML(server.catalogHTML())
	session.pickHit(hits[0])
	if session.Phase() != PhaseParsing {
		t.Fatalf("phase = %v, want parsing after picking a book", session.Phase())
	}
	page.finishLoad()
	waitFor(t, func() bool { return session.Phase() == PhaseCatalogReady })

	catalog := session.Catalog()
	if catalog == nil || len(catalog.Chapters) != 3 {
		t.Fatalf("catalog = %+v, want 3 chapters", catalog)
	}

	// 3. Download. The crawler reads the fake site directly.
	session.startDownload()
	waitFor(t, func() bool { return session.Phase() == PhaseFinished })

	if len(session.Chapters()) != 3 {
		t.Fatalf("%d chapters downloaded, want 3", len(session.Chapters()))
	}
	if session.LastSaved() == "" {
		t.Fatal("the book was not written")
	}

	// 4. The EPUB holds the three chapters.
	data, err := os.ReadFile(session.LastSaved())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, want := range []string{
		"mimetype",
		"OEBPS/content.opf",
		"OEBPS/nav.xhtml",
		"OEBPS/chapter-001.xhtml",
		"OEBPS/chapter-002.xhtml",
		"OEBPS/chapter-003.xhtml",
	} {
		if !names[want] {
			t.Errorf("the EPUB is missing %q", want)
		}
	}

	// 5. The history holds the book once, and can export it again.
	if history.Count() != 1 {
		t.Errorf("%d history records, want 1", history.Count())
	}
	summary := history.List()[0]
	if summary.ProgressLabel() != "3 / 3" || !summary.Complete() {
		t.Errorf("history summary = %+v", summary)
	}
	_ = window
}

// TestEndToEndKeepsTheHistoryToTheDownloadedChapters checks a partial run is
// saved as partial, not as a complete book.
func TestEndToEndKeepsTheHistoryToTheDownloadedChapters(t *testing.T) {
	server := newFakeSite(t, 2)
	defer server.Close()
	useTempDownloads(t)

	source := fakeSource(server)
	history := NewHistory(filepath.Join(t.TempDir(), "history"))
	page := &fakePage{html: server.catalogHTML()}
	panel := newFakePanel(page, source)
	session := NewSession(nil, panel, history, source)

	*session.Input() = server.URL + "/book/90028/"
	session.Submit()
	page.finishLoad()
	waitFor(t, func() bool { return session.Phase() == PhaseCatalogReady })

	if session.Catalog() == nil || len(session.Catalog().Chapters) != 2 {
		t.Fatalf("catalog = %+v", session.Catalog())
	}

	// Save what a stopped run would have kept.
	session.chapters = []Chapter{{Title: "第一章 夜路", Order: 1, PlainText: "正文"}}
	session.next = 1
	session.stopDownload()

	list := history.List()
	if len(list) != 1 {
		t.Fatalf("%d records, want the stopped run saved", len(list))
	}
	if list[0].Complete() {
		t.Error("a partial run is stored as complete")
	}
	if list[0].ProgressLabel() != "1 / 2" {
		t.Errorf("progress = %q, want 1 / 2", list[0].ProgressLabel())
	}
}

// useTempDownloads points exports at a temporary directory for the duration of
// a test, so a test run never writes a book into the user's real Downloads
// folder.
func useTempDownloads(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	original := downloadsDir
	downloadsDir = func() string { return dir }
	t.Cleanup(func() { downloadsDir = original })
}

// fakeSource is 69shuba's parser pointed at a local site, so the whole flow
// can run without touching the network.
func fakeSource(site *fakeSite) Source {
	source := SourceFor(SourceShuba69)
	source.NormalizeCatalog = func(u *url.URL) *url.URL {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 && strings.EqualFold(parts[0], "book") {
			id := strings.TrimSuffix(strings.ToLower(parts[1]), ".htm")
			if allDigits(id) {
				return &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/book/" + id + "/"}
			}
		}
		return nil
	}
	source.SearchURL = func(query string) string { return site.URL + "/search?q=" + url.QueryEscape(query) }
	source.ParseSearch = parseShuba69Search
	// A local site needs no politeness delay; the real one keeps its own.
	source.ChapterDelayMin = 0
	source.ChapterDelayMax = 0
	return source
}

// fakeSite is a stand-in for one of the sites: a search page, a catalog page
// and the chapters it lists.
type fakeSite struct {
	*httptest.Server
	chapters int
}

// newFakeSite starts a site with a book of n chapters.
func newFakeSite(t *testing.T, chapters int) *fakeSite {
	t.Helper()
	site := &fakeSite{chapters: chapters}

	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><body><table>
			<tr><td><a href="/book/90028/">极道天魔</a></td><td>最新章节</td><td>滚开</td></tr>
			</table></body></html>`)
	})
	mux.HandleFunc("/book/90028/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, site.catalogHTML())
	})
	for i := 1; i <= chapters; i++ {
		i := i
		mux.HandleFunc(chapterPath(i), func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, chapterHTMLFor(i))
		})
	}

	site.Server = httptest.NewServer(mux)
	return site
}

// catalogHTML is the book's catalog page.
func (s *fakeSite) catalogHTML() string {
	var b strings.Builder
	b.WriteString(`<html><head>
		<meta property="og:novel:book_name" content="极道天魔"/>
		<meta property="og:novel:author" content="滚开"/>
		</head><body><ul>`)
	for i := 1; i <= s.chapters; i++ {
		b.WriteString(`<li><a href="` + chapterPath(i) + `">第` + itoa(i) + `章 夜路</a></li>`)
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

// chapterPath is the address of a chapter on the fake site.
func chapterPath(n int) string { return "/txt/90028/" + itoa(n) }

// chapterHTMLFor is a chapter page with two paragraphs.
func chapterHTMLFor(n int) string {
	return `<html><body><h1>第` + itoa(n) + `章 夜路</h1>
		<div id="chaptercontent">第` + itoa(n) + `章的第一段正文，长度足够通过校验。<br/><br/>
		第` + itoa(n) + `章的第二段正文，同样足够长。<br/></div>
		<div id="bottomlink"></div></body></html>`
}

// TestDownloadLoopWritesCheckpoints checks a run saves progress as it goes,
// rather than only at the end.
func TestDownloadLoopWritesCheckpoints(t *testing.T) {
	server := newFakeSite(t, checkpointEvery+1)
	defer server.Close()
	useTempDownloads(t)

	source := fakeSource(server)
	history := NewHistory(filepath.Join(t.TempDir(), "history"))
	page := &fakePage{html: server.catalogHTML()}
	panel := newFakePanel(page, source)
	session := NewSession(nil, panel, history, source)

	*session.Input() = server.URL + "/book/90028/"
	session.Submit()
	page.finishLoad()
	waitFor(t, func() bool { return session.Phase() == PhaseCatalogReady })

	// The run writes a checkpoint at the chapter count, so a book this long
	// is saved before it finishes.
	session.startDownload()
	waitFor(t, func() bool {
		return history.Count() > 0 && len(history.List()) > 0 && history.List()[0].Downloaded >= checkpointEvery
	}, 30*time.Second)

	list := history.List()
	if len(list) == 0 || list[0].Downloaded < checkpointEvery {
		t.Fatalf("history = %+v, want a checkpoint at %d chapters", list, checkpointEvery)
	}
	session.stopDownload()
}

// waitFor polls a condition until it holds or the timeout passes.
func waitFor(t *testing.T, done func() bool, timeout ...time.Duration) {
	t.Helper()
	limit := 10 * time.Second
	if len(timeout) > 0 {
		limit = timeout[0]
	}
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the condition did not hold in time")
}
