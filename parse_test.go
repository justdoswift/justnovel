package main

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".html")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestShuba69Catalog(t *testing.T) {
	base := mustURL(t, "https://www.69shuba.com/book/27659.htm")
	catalog, err := shuba69Adapter{}.ParseCatalog(fixture(t, "catalog"), base)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Title != "极道天魔" {
		t.Errorf("title = %q, want 极道天魔", catalog.Title)
	}
	if catalog.Author != "滚开" {
		t.Errorf("author = %q, want 滚开", catalog.Author)
	}
	if len(catalog.Chapters) != 3 {
		t.Fatalf("%d chapters, want 3", len(catalog.Chapters))
	}
	if got := catalog.Chapters[1].URL; got != "https://www.69shuba.com/txt/27659/37954871" {
		t.Errorf("second chapter URL = %q", got)
	}
	if got := catalog.Chapters[1].Order; got != 2 {
		t.Errorf("second chapter order = %d, want 2", got)
	}
}

func TestShuba69Chapter(t *testing.T) {
	chapter, err := shuba69Adapter{}.ParseChapter(fixture(t, "chapter"), mustURL(t, "https://www.69shuba.com/txt/27659/37954870"))
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Title != "第一章 夜路" {
		t.Errorf("title = %q", chapter.Title)
	}
	paragraphs := strings.Split(chapter.PlainText, "\n\n")
	if len(paragraphs) != 3 {
		t.Fatalf("paragraphs = %q", paragraphs)
	}
	if paragraphs[2] != "第三段内容。" {
		t.Errorf("last paragraph = %q", paragraphs[2])
	}
	for _, p := range paragraphs {
		if strings.Contains(p, "69shuba.com") {
			t.Errorf("advertisement kept: %q", p)
		}
	}
}

func TestShuba69TxtnavChapter(t *testing.T) {
	chapter, err := shuba69Adapter{}.ParseChapter(fixture(t, "chapter-txtnav"), mustURL(t, "https://www.69shuba.com/txt/90028/41039597"))
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Title != "第548章 大战（求月票！）" {
		t.Errorf("title = %q", chapter.Title)
	}
	if !strings.Contains(chapter.PlainText, "霜寂法王眼中瞬间爆发出狂喜的精光") {
		t.Errorf("prose missing: %q", chapter.PlainText)
	}
	for _, noise := range []string{"&emsp;", "首页", "loadAdv", "txtinfo"} {
		if strings.Contains(chapter.PlainText, noise) {
			t.Errorf("noise %q kept in %q", noise, chapter.PlainText)
		}
	}
}

func TestPiaotianCatalog(t *testing.T) {
	base := mustURL(t, "https://www.piaotia.com/html/15/15709/")
	catalog, err := piaotianAdapter{}.ParseCatalog(fixture(t, "piaotian-catalog"), base)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Title != "天人图谱" {
		t.Errorf("title = %q, want 天人图谱", catalog.Title)
	}
	if catalog.Author != "误道者" {
		t.Errorf("author = %q, want 误道者", catalog.Author)
	}
	if len(catalog.Chapters) != 2 {
		t.Fatalf("chapters = %v, want 2", catalog.Chapters)
	}
	if got := catalog.Chapters[0].URL; got != "https://www.piaotia.com/html/15/15709/11428790.html" {
		t.Errorf("first chapter URL = %q", got)
	}
}

func TestPiaotianChapter(t *testing.T) {
	chapter, err := piaotianAdapter{}.ParseChapter(fixture(t, "piaotian-chapter"), mustURL(t, "https://www.piaotia.com/html/15/15709/11428792.html"))
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Title != "第3章 前世今生皆为我" {
		t.Errorf("title = %q", chapter.Title)
	}
	paragraphs := strings.Split(chapter.PlainText, "\n\n")
	if len(paragraphs) != 3 {
		t.Fatalf("paragraphs = %q", paragraphs)
	}
	if !strings.HasPrefix(paragraphs[0], "单雄虽然人品差") {
		t.Errorf("first paragraph = %q", paragraphs[0])
	}
}

func TestSto9CatalogNeedsWholeList(t *testing.T) {
	base := mustURL(t, "https://sto9.com/book/13564/index.html")
	gapped := `<div id="allchapter"><ul>
	    <li data-num="1"><a href="https://sto9.com/txt/13564/7388646.html">001覺醒</a></li>
	    <li data-num="3"><a href="https://sto9.com/txt/13564/8169182.html">第3章 終章</a></li>
	</ul></div>`
	if _, err := (sto9Adapter{}).ParseCatalog(gapped, base); err == nil {
		t.Fatal("a catalog with a gap parsed, want an error")
	}

	whole := fixture(t, "sto9-catalog") + fixture(t, "sto9-chapter-list")
	catalog, err := sto9Adapter{}.ParseCatalog(whole, base)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Title != "腐朽世界" {
		t.Errorf("title = %q, want 腐朽世界", catalog.Title)
	}
	if len(catalog.Chapters) != 3 {
		t.Fatalf("chapters = %v, want 3", catalog.Chapters)
	}
	for i, chapter := range catalog.Chapters {
		if chapter.Order != i+1 {
			t.Errorf("chapter %d order = %d", i, chapter.Order)
		}
	}
}

func TestSto9Chapter(t *testing.T) {
	chapter, err := sto9Adapter{}.ParseChapter(fixture(t, "sto9-chapter"), mustURL(t, "https://sto9.com/txt/13564/8169182.html"))
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Title != "第608章 608大结局 下" {
		t.Errorf("title = %q", chapter.Title)
	}
	if !strings.Contains(chapter.PlainText, "广袤的灰褐色大地上，林辉静静漫步著。") {
		t.Errorf("traditional text not converted: %q", chapter.PlainText)
	}
	for _, noise := range []string{"sto9.com", "還有更新耶", "loadAdv", "&gt;"} {
		if strings.Contains(chapter.PlainText, noise) {
			t.Errorf("noise %q kept", noise)
		}
	}
}

func TestCatalogURLNormalization(t *testing.T) {
	sto9 := SourceFor(SourceSto9)
	for _, tc := range []struct{ in, want string }{
		{"https://sto9.com/book/13564/index.html", "https://sto9.com/book/13564/index.html"},
		{"https://sto9.com/book/13564.html", "https://sto9.com/book/13564/index.html"},
		{"https://sto9.com/txt/13564/8169182.html", "https://sto9.com/book/13564/index.html"},
	} {
		got := sto9.CatalogURL(tc.in)
		if got == nil || got.String() != tc.want {
			t.Errorf("CatalogURL(%q) = %v, want %s", tc.in, got, tc.want)
		}
	}
	for _, in := range []string{"https://sto9.com/txt/13564/end.html", "https://notsto9.com/txt/13564/8169182.html"} {
		if got := sto9.CatalogURL(in); got != nil {
			t.Errorf("CatalogURL(%q) = %v, want nil", in, got)
		}
	}

	if got := SourceFor(SourceShuba69).CatalogURL("https://www.69shuba.com/book/90028.htm"); got == nil || got.String() != "https://www.69shuba.com/book/90028/" {
		t.Errorf("69shuba book page normalized to %v, want the full catalog", got)
	}
	if got := SourceFor(SourceShuba69).CatalogURL("https://www.69shuba.com/book/90028/"); got == nil || got.String() != "https://www.69shuba.com/book/90028/" {
		t.Errorf("69shuba catalog URL = %v", got)
	}
	if got := SourceFor(SourceShuba69).CatalogURL("https://www.piaotia.com/html/15/15709/"); got != nil {
		t.Error("piaotian URL accepted by 69shuba")
	}
	if got := SourceFor(SourcePiaotian).CatalogURL("https://www.piaotia.com/html/15/15709/"); got == nil {
		t.Error("piaotian catalog URL rejected")
	}
	if got := SourceFor(SourcePiaotian).CatalogURL("https://www.piaotia.com/html/15/15709/11428790.html"); got != nil {
		t.Error("piaotian chapter URL accepted as a catalog")
	}
}
