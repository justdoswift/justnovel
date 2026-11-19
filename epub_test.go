package main

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestBuildEPUBStructure(t *testing.T) {
	settings := ExportSettings{
		Title:      "极道天魔",
		Author:     "滚开",
		SourceURL:  "https://www.69shuba.com/book/90028/",
		Total:      2,
		BodyFont:   FontSongti,
		FrontText:  "第一段说明。\n\n第二段说明。",
		ShowSource: true,
		CoverImage: []byte("png-bytes"),
		CoverType:  "image/png",
		CoverExt:   "png",
	}
	chapters := []Chapter{
		{Title: "第一章 夜路", PlainText: "第一段。<script>\n\n第二段 & 更多。", Order: 1},
		{Title: "第二章 敲门", PlainText: "第三段。", Order: 2},
	}

	data, err := BuildEPUB(settings, chapters)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	first := reader.File[0]
	if first.Name != "mimetype" {
		t.Fatalf("first entry = %q, want mimetype", first.Name)
	}
	if first.Method != zip.Store {
		t.Errorf("mimetype method = %d, want stored", first.Method)
	}
	if got := readZip(t, first); got != "application/epub+zip" {
		t.Errorf("mimetype = %q", got)
	}

	chapter := readZipFile(t, reader, "OEBPS/chapter-001.xhtml")
	if !strings.Contains(chapter, "<p>第一段。&lt;script&gt;</p>") {
		t.Errorf("chapter text not escaped: %q", chapter)
	}
	if !strings.Contains(chapter, "<p>第二段 &amp; 更多。</p>") {
		t.Errorf("ampersand not escaped: %q", chapter)
	}

	nav := readZipFile(t, reader, "OEBPS/nav.xhtml")
	for _, want := range []string{"第一章 夜路", "第二章 敲门", `href="chapter-001.xhtml"`} {
		if !strings.Contains(nav, want) {
			t.Errorf("nav missing %q: %q", want, nav)
		}
	}

	opf := readZipFile(t, reader, "OEBPS/content.opf")
	for _, want := range []string{"<dc:title>极道天魔</dc:title>", `<item id="cover-image" href="cover.png" media-type="image/png"`, `idref="chapter-2"`} {
		if !strings.Contains(opf, want) {
			t.Errorf("package document missing %q: %q", want, opf)
		}
	}
	if got := readZipFile(t, reader, "OEBPS/cover.png"); got != "png-bytes" {
		t.Errorf("cover = %q", got)
	}

	title := readZipFile(t, reader, "OEBPS/title.xhtml")
	if !strings.Contains(title, "来源：https://www.69shuba.com/book/90028/") {
		t.Errorf("title page lacks the source: %q", title)
	}
	if !strings.Contains(title, "第二段说明。") {
		t.Errorf("title page lacks its body text: %q", title)
	}
}

func TestExportFileName(t *testing.T) {
	if got := ExportFileName("极道天魔", 567, 567); got != "极道天魔.epub" {
		t.Errorf("complete name = %q", got)
	}
	if got := ExportFileName("极道天魔", 12, 567); got != "极道天魔-partial-12-of-567.epub" {
		t.Errorf("partial name = %q", got)
	}
	if got := ExportFileName("a/b:c", 1, 1); got != "a-b-c.epub" {
		t.Errorf("unsafe name = %q", got)
	}
}

func readZip(t *testing.T, file *zip.File) string {
	t.Helper()
	r, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readZipFile(t *testing.T, reader *zip.Reader, name string) string {
	t.Helper()
	for _, file := range reader.File {
		if file.Name == name {
			return readZip(t, file)
		}
	}
	t.Fatalf("%s is missing from the archive", name)
	return ""
}
