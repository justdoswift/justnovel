package main

import (
	"net/url"
	"testing"
)

// TestPiaotianSearchURLIsGBK checks the query is encoded the way the site
// reads it. A UTF-8 query matches nothing there, which is why searching used
// to come back empty.
func TestPiaotianSearchURLIsGBK(t *testing.T) {
	got := piaotianSearchURL("天人")
	want := "https://www.piaotia.com/modules/article/search.php?searchkey=%CC%EC%C8%CB"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

// TestPiaotianResultRows reads a results table: one row per book, the name in
// a /bookinfo/ link, the author in the third cell.
func TestPiaotianResultRows(t *testing.T) {
	page := `<table class="grid">
	<tr><th>文章名称</th><th>最新章节</th><th>作者</th></tr>
	<tr>
		<td class="odd"><a href="/bookinfo/15/15709.html">天人图谱</a></td>
		<td class="even"><a href="/html/15/15709/index.html">第2099章 番外二</a></td>
		<td class="odd">误道者</td>
	</tr>
	<tr>
		<td class="odd"><a href="/bookinfo/7/7591.html">极道天人</a></td>
		<td class="even"><a href="/html/7/7591/index.html">大结局</a></td>
		<td class="odd">飘渺的冰蓝</td>
	</tr>
	</table>`

	base, _ := url.Parse("https://www.piaotia.com/modules/article/search.php")
	hits := parsePiaotianSearch(page, base)
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].Title != "天人图谱" || hits[0].Author != "误道者" {
		t.Errorf("first hit = %q by %q", hits[0].Title, hits[0].Author)
	}
	if want := "https://www.piaotia.com/bookinfo/15/15709.html"; hits[0].URL != want {
		t.Errorf("url = %q, want %q", hits[0].URL, want)
	}
}

// TestPiaotianBookPage reads the page a search lands on when it matched one
// book: the site redirects there instead of listing results.
func TestPiaotianBookPage(t *testing.T) {
	page := `<html><head><title>极道天魔最新章节,滚开作品选-飘天文学</title></head>
	<body>
	<h1>极道天魔</h1>
	<table><tr>
		<td width="25%">类&nbsp;&nbsp;&nbsp; 别：武侠修真</td>
		<td width="25%">作&nbsp;&nbsp;&nbsp; 者：滚开</td>
	</tr></table>
	<a title="点击阅读《极道天魔》" href="https://www.piaotia.com/html/8/8502/"><img src="/images/dian.gif"></a>
	</body></html>`

	base, _ := url.Parse("https://www.piaotia.com/modules/article/search.php")
	hits := parsePiaotianSearch(page, base)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].Title != "极道天魔" {
		t.Errorf("title = %q, want 极道天魔", hits[0].Title)
	}
	if hits[0].Author != "滚开" {
		t.Errorf("author = %q, want 滚开", hits[0].Author)
	}
	if want := "https://www.piaotia.com/html/8/8502/"; hits[0].URL != want {
		t.Errorf("url = %q, want %q", hits[0].URL, want)
	}
}

// TestSto9SearchForm checks the search is posted as the site's own form posts
// it. A GET to the same address returns an empty page.
func TestSto9SearchForm(t *testing.T) {
	form := sto9SearchForm("斗破苍穹")
	if got := form.Get("searchkey"); got != "斗破苍穹" {
		t.Errorf("searchkey = %q", got)
	}
	if got := form.Get("searchtype"); got != "all" {
		t.Errorf("searchtype = %q, want all", got)
	}
}

// TestSto9SearchResults reads a results page: each book is an h3 link to
// /book/<id>.html, with the author in the first label under it.
func TestSto9SearchResults(t *testing.T) {
	page := `<ul id="article_list_content">
	<li>
		<a href="https://sto9.com/book/15270.html" class="imgbox"><img src="/x.jpg"></a>
		<div class="newnav">
			<h3><a href="https://sto9.com/book/15270.html"><span class="hottext">斗破蒼穹</span>之雷霆震碎</a></h3>
			<div class="labelbox"><label>一貧如洗羊羊</label><label>其他類型</label></div>
		</div>
	</li>
	</ul>`

	base, _ := url.Parse("https://sto9.com/search")
	hits := parseSto9Search(page, base)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].Title != "斗破蒼穹之雷霆震碎" {
		t.Errorf("title = %q", hits[0].Title)
	}
	if hits[0].Author != "一貧如洗羊羊" {
		t.Errorf("author = %q", hits[0].Author)
	}
}

// TestShuba69SearchResults reads 69shuba's results table.
func TestShuba69SearchResults(t *testing.T) {
	page := `<table class="grid">
	<tr><th>书名</th><th>最新章节</th><th>作者</th><th>字数</th></tr>
	<tr>
		<td><a href="/book/90028/">极道天魔</a></td>
		<td><a href="/txt/90028/1.html">第一章</a></td>
		<td>滚开</td>
		<td>610万字</td>
	</tr>
	</table>`

	hits := parseShuba69Search(page, nil)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].Title != "极道天魔" || hits[0].Author != "滚开" {
		t.Errorf("hit = %q by %q", hits[0].Title, hits[0].Author)
	}
}

// TestPiaotianBookPageResolvesToCatalog checks a book page from a search turns
// into the catalog the downloader reads.
func TestPiaotianBookPageResolvesToCatalog(t *testing.T) {
	source := SourceFor(SourcePiaotian)
	got := source.CatalogURL("https://www.piaotia.com/bookinfo/15/15709.html")
	if got == nil {
		t.Fatal("a book page did not resolve")
	}
	if want := "https://www.piaotia.com/html/15/15709/"; got.String() != want {
		t.Errorf("catalog = %q, want %q", got.String(), want)
	}
}
