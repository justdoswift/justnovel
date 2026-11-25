package main

import (
	"net/url"
	"strings"
	"testing"
)

// These tests read pages captured from the live sites, so a change in a site's
// markup shows up here rather than in the app. The fixtures under testdata are
// real responses, not hand-written HTML: 极道天魔 on 飘天文学, its catalog and
// its first chapter.
//
// They exist because the hand-written fixtures passed while the real site did
// not: the catalog parser matched a wrapper div that holds nested divs, so it
// captured 116 characters of markup and found no chapters at all.

func realFixture(t *testing.T, name string) string {
	t.Helper()
	return fixture(t, name)
}

// TestPiaotianRealCatalogReadsEveryChapter is the regression for a catalog
// that came back empty against the live site.
func TestPiaotianRealCatalogReadsEveryChapter(t *testing.T) {
	base := mustURL(t, "https://www.piaotia.com/html/8/8502/")
	catalog, err := piaotianAdapter{}.ParseCatalog(realFixture(t, "piaotian-real-catalog"), base)
	if err != nil {
		t.Fatal(err)
	}

	if catalog.Title != "极道天魔" {
		t.Errorf("title = %q, want 极道天魔", catalog.Title)
	}
	if catalog.Author != "滚开" {
		t.Errorf("author = %q, want 滚开", catalog.Author)
	}
	if len(catalog.Chapters) != 1214 {
		t.Fatalf("%d chapters, want the 1214 the page lists", len(catalog.Chapters))
	}

	first := catalog.Chapters[0]
	if first.Title != "第一章 道法无情" || first.Order != 1 {
		t.Errorf("first chapter = %+v", first)
	}
	if first.URL != "https://www.piaotia.com/html/8/8502/5379303.html" {
		t.Errorf("first chapter URL = %q", first.URL)
	}

	last := catalog.Chapters[len(catalog.Chapters)-1]
	if !strings.Contains(last.Title, "终章") || last.Order != 1214 {
		t.Errorf("last chapter = %+v", last)
	}

	// The site's own navigation must not be taken for a chapter.
	for _, chapter := range catalog.Chapters {
		for _, chrome := range []string{"返回目录", "上一章", "下一章", "加入书架", "推荐本书"} {
			if strings.Contains(chapter.Title, chrome) {
				t.Fatalf("site chrome %q parsed as a chapter: %+v", chrome, chapter)
			}
		}
	}
}

// TestPiaotianRealChapterReadsTheProse checks the chapter body comes out as
// paragraphs, with the site's chrome stripped.
func TestPiaotianRealChapterReadsTheProse(t *testing.T) {
	base := mustURL(t, "https://www.piaotia.com/html/8/8502/5379303.html")
	chapter, err := piaotianAdapter{}.ParseChapter(realFixture(t, "piaotian-real-chapter"), base)
	if err != nil {
		t.Fatal(err)
	}

	if chapter.Title != "第一章 道法无情" {
		t.Errorf("title = %q", chapter.Title)
	}
	if !strings.HasPrefix(chapter.PlainText, "冷风如刀，大雪纷飞。") {
		t.Errorf("chapter does not start with the prose: %q", firstRunes(chapter.PlainText, 40))
	}
	if runes := len([]rune(chapter.PlainText)); runes < 3000 {
		t.Errorf("chapter is %d runes, want the whole chapter", runes)
	}
	for _, chrome := range []string{"上一章", "下一章", "返回目录", "加入书架", "PT文学"} {
		if strings.Contains(chapter.PlainText, chrome) {
			t.Errorf("site chrome %q left in the chapter", chrome)
		}
	}
	// Paragraphs are separated by a blank line, which is what the EPUB writer
	// splits on.
	if !strings.Contains(chapter.PlainText, "\n\n") {
		t.Error("the chapter has no paragraph breaks")
	}
}

// TestPiaotianRealCatalogURLsResolve checks every chapter URL is absolute and
// on the site, which the download loop relies on.
func TestPiaotianRealCatalogURLsResolve(t *testing.T) {
	base := mustURL(t, "https://www.piaotia.com/html/8/8502/")
	catalog, err := piaotianAdapter{}.ParseCatalog(realFixture(t, "piaotian-real-catalog"), base)
	if err != nil {
		t.Fatal(err)
	}
	for i, chapter := range catalog.Chapters {
		u, err := url.Parse(chapter.URL)
		if err != nil || u.Host == "" || u.Scheme == "" {
			t.Fatalf("chapter %d has a relative URL: %q", i+1, chapter.URL)
		}
		if !strings.Contains(u.Host, "piaotia.com") {
			t.Fatalf("chapter %d points off-site: %q", i+1, chapter.URL)
		}
	}
}

// firstRunes is the first n runes of a string, for a readable failure.
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
