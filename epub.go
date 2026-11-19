package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// BuildEPUB writes the book as an EPUB 3 archive.
func BuildEPUB(settings ExportSettings, chapters []Chapter) ([]byte, error) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)

	// The mimetype entry must come first and stay uncompressed.
	mimetype, err := archive.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := mimetype.Write([]byte("application/epub+zip")); err != nil {
		return nil, err
	}

	files := []struct{ name, body string }{
		{"META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`},
		{"OEBPS/stylesheet.css", stylesheet(settings)},
		{"OEBPS/title.xhtml", xhtmlDocument(settings.Title, titlePage(settings))},
		{"OEBPS/nav.xhtml", xhtmlDocument("目录", navigation(chapters))},
		{"OEBPS/content.opf", packageDocument(settings, chapters)},
	}
	for _, file := range files {
		if err := writeFile(archive, file.name, []byte(file.body)); err != nil {
			return nil, err
		}
	}

	if len(settings.CoverImage) > 0 {
		if err := writeFile(archive, "OEBPS/"+coverFileName(settings), settings.CoverImage); err != nil {
			return nil, err
		}
	}

	for _, chapter := range chapters {
		body := "<h2>" + xmlEscaped(chapter.Title) + "</h2>\n" + chapterHTML(chapter)
		if err := writeFile(archive, "OEBPS/"+chapterFileName(chapter.Order), []byte(xhtmlDocument(chapter.Title, body))); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeFile(archive *zip.Writer, name string, body []byte) error {
	file, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write(body)
	return err
}

func stylesheet(settings ExportSettings) string {
	return fmt.Sprintf(`body { font-family: %s; line-height: 1.8; margin: 5%%; }
h1, h2 { line-height: 1.3; }
p { margin: 0 0 1em; text-indent: 2em; }
nav ol { padding-left: 1.2em; }
a { color: inherit; text-decoration: none; }
.meta { color: #666666; margin-bottom: 1.5em; }
.title-page { text-align: center; }
.title-page h1, .title-page .meta { text-indent: 0; }
.cover-image { display: block; max-width: min(100%%, 28rem); max-height: 70vh; margin: 0 auto 2rem; border-radius: 0.75rem; }
`, settings.BodyFont.CSSFamily())
}

func titlePage(settings ExportSettings) string {
	var b strings.Builder
	b.WriteString(`<section class="title-page">`)
	if len(settings.CoverImage) > 0 {
		b.WriteString(`<img class="cover-image" src="` + xmlEscaped(coverFileName(settings)) + `" alt="` + xmlEscaped(settings.Title) + ` 封面"/>`)
	}
	b.WriteString("<h1>" + xmlEscaped(settings.Title) + "</h1>")
	b.WriteString(`<p class="meta">作者：` + xmlEscaped(settings.Author) + `</p>`)
	if settings.ShowSource {
		b.WriteString(`<p class="meta">来源：` + xmlEscaped(settings.SourceURL) + `</p>`)
	}
	for _, paragraph := range strings.Split(strings.ReplaceAll(settings.FrontText, "\r\n", "\n"), "\n\n") {
		if paragraph = trimParagraph(paragraph); paragraph != "" {
			b.WriteString(`<p>` + xmlEscaped(paragraph) + `</p>`)
		}
	}
	b.WriteString(`</section>`)
	return b.String()
}

func navigation(chapters []Chapter) string {
	var b strings.Builder
	b.WriteString(`<nav epub:type="toc" id="toc"><h2>目录</h2><ol><li><a href="title.xhtml">封面页</a></li>`)
	for _, chapter := range chapters {
		b.WriteString(`<li><a href="` + chapterFileName(chapter.Order) + `">` + xmlEscaped(chapter.Title) + `</a></li>`)
	}
	b.WriteString(`</ol></nav>`)
	return b.String()
}

func packageDocument(settings ExportSettings, chapters []Chapter) string {
	manifest := []string{
		`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`,
		`<item id="style" href="stylesheet.css" media-type="text/css"/>`,
		`<item id="title" href="title.xhtml" media-type="application/xhtml+xml"/>`,
	}
	if len(settings.CoverImage) > 0 {
		manifest = append(manifest, fmt.Sprintf(`<item id="cover-image" href="%s" media-type="%s" properties="cover-image"/>`,
			xmlEscaped(coverFileName(settings)), xmlEscaped(settings.CoverType)))
	}
	spine := []string{`<itemref idref="title"/>`}
	for _, chapter := range chapters {
		manifest = append(manifest, fmt.Sprintf(`<item id="chapter-%d" href="%s" media-type="application/xhtml+xml"/>`,
			chapter.Order, chapterFileName(chapter.Order)))
		spine = append(spine, fmt.Sprintf(`<itemref idref="chapter-%d"/>`, chapter.Order))
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="book-id" xml:lang="zh-CN">` + "\n")
	b.WriteString(`  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	b.WriteString(`    <dc:identifier id="book-id">urn:uuid:` + newUUID() + `</dc:identifier>` + "\n")
	b.WriteString(`    <dc:title>` + xmlEscaped(settings.Title) + `</dc:title>` + "\n")
	b.WriteString(`    <dc:creator>` + xmlEscaped(settings.Author) + `</dc:creator>` + "\n")
	b.WriteString(`    <dc:language>zh-CN</dc:language>` + "\n")
	if len(settings.CoverImage) > 0 {
		b.WriteString(`    <meta name="cover" content="cover-image"/>` + "\n")
	}
	b.WriteString(`    <meta property="dcterms:modified">` + time.Now().UTC().Format("2006-01-02T15:04:05Z") + `</meta>` + "\n")
	b.WriteString(`  </metadata>` + "\n")
	b.WriteString("  <manifest>\n    " + strings.Join(manifest, "\n    ") + "\n  </manifest>\n")
	b.WriteString("  <spine>\n    " + strings.Join(spine, "\n    ") + "\n  </spine>\n")
	b.WriteString(`</package>`)
	return b.String()
}

func xhtmlDocument(title, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="zh-CN" xml:lang="zh-CN">
  <head>
    <title>` + xmlEscaped(title) + `</title>
    <link rel="stylesheet" type="text/css" href="stylesheet.css"/>
  </head>
  <body>
    ` + body + `
  </body>
</html>`
}

func chapterFileName(order int) string {
	return fmt.Sprintf("chapter-%03d.xhtml", order)
}

func coverFileName(settings ExportSettings) string {
	ext := trimParagraph(settings.CoverExt)
	if ext == "" {
		return "cover.png"
	}
	return "cover." + ext
}

// ExportFileName is the name the save dialog proposes.
func ExportFileName(title string, exported, total int) string {
	name := safeFileName(title, "JustNovel")
	if exported >= total {
		return name + ".epub"
	}
	return fmt.Sprintf("%s-partial-%d-of-%d.epub", name, exported, total)
}

// coverTypeFor names the media type and extension of a cover image.
func coverTypeFor(path string) (mediaType, ext string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg", "jpg"
	case ".webp":
		return "image/webp", "webp"
	default:
		return "image/png", "png"
	}
}
