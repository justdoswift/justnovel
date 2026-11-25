package main

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// shuba69Adapter reads 69shuba's catalog and chapter pages.
type shuba69Adapter struct{}

func (shuba69Adapter) ParseCatalog(html string, base *url.URL) (BookCatalog, error) {
	page := decodeEntities(html)
	title := shuba69Title(page)
	author := shuba69Author(page)
	chapters := shuba69Chapters(page, base)

	if title == "" {
		return BookCatalog{}, errors.New("目录页里没有解析到书名。")
	}
	if len(chapters) == 0 {
		return BookCatalog{}, errors.New("目录页里没有找到章节链接，可能站点结构有变化。")
	}

	return BookCatalog{
		Title:     title,
		Author:    orUnknown(author),
		SourceURL: base.String(),
		Chapters:  chapters,
	}, nil
}

func (shuba69Adapter) ParseChapter(html string, page *url.URL) (Chapter, error) {
	doc := decodeEntities(html)
	title := shuba69ChapterTitle(doc)
	paragraphs := removeLeadingTitles(shuba69Paragraphs(doc, title), title, identityTitle)
	if len(paragraphs) == 0 {
		return Chapter{}, errors.New("章节页 " + page.String() + " 抓到的不是正文页，或没有解析到有效正文。")
	}
	return chapterFrom(title, paragraphs), nil
}

func (shuba69Adapter) SupplementURLs(*url.URL) []*url.URL { return nil }
func (shuba69Adapter) MetadataURLs(*url.URL) []*url.URL   { return nil }

func shuba69Title(html string) string {
	patterns := []string{
		`<meta[^>]+property=["']og:novel:book_name["'][^>]+content=["']([^"']+)["']`,
		`<h1[^>]*>(.*?)</h1>`,
		`<title>(.*?)</title>`,
	}
	for _, pattern := range patterns {
		if m, ok := firstMatch(pattern, html); ok {
			title := trimParagraph(decodeEntities(stripTags(m)))
			if title != "" {
				return trimParagraph(strings.ReplaceAll(title, "最新章节", ""))
			}
		}
	}
	return ""
}

func shuba69Author(html string) string {
	patterns := []string{
		`<meta[^>]+property=["']og:novel:author["'][^>]+content=["']([^"']+)["']`,
		`作者[：: ]*</span>\s*<a[^>]*>(.*?)</a>`,
		`作者[：: ]*</span>\s*([^<\n]+)`,
		`作\s*者[：: ]*([^<\n]+)`,
	}
	for _, pattern := range patterns {
		if m, ok := firstMatch(pattern, html); ok {
			if author := trimParagraph(decodeEntities(stripTags(m))); author != "" {
				return author
			}
		}
	}
	return ""
}

func shuba69Chapters(html string, base *url.URL) []ChapterRef {
	matches := allMatches(`<a[^>]+href=["']([^"']*(?:/txt/\d+/\d+[^"']*))["'][^>]*>(.*?)</a>`, html)
	seen := map[string]bool{}
	var chapters []ChapterRef
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		href := trimParagraph(m[0])
		title := trimParagraph(decodeEntities(stripTags(m[1])))
		if title == "" || looksLikeNavigation(title) {
			continue
		}
		resolved := resolveURL(base, href)
		if resolved == "" || seen[resolved] {
			continue
		}
		seen[resolved] = true
		chapters = append(chapters, ChapterRef{Title: title, URL: resolved, Order: len(chapters) + 1})
	}
	return chapters
}

func looksLikeNavigation(title string) bool {
	lowered := strings.ToLower(title)
	for _, token := range []string{"上一章", "下一章", "返回目录", "加入书签"} {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return false
}

func shuba69ChapterTitle(html string) string {
	patterns := []string{
		`chaptername\s*:\s*'([^']+)'`,
		`<meta[^>]+property=["']og:novel:chapter_name["'][^>]+content=["']([^"']+)["']`,
		`<div[^>]+class=["'][^"']*txtnav[^"']*["'][^>]*>.*?<h1[^>]*>(.*?)</h1>`,
		`<h1[^>]*>(.*?)</h1>`,
		`<title>(.*?)</title>`,
	}
	for _, pattern := range patterns {
		if m, ok := firstMatch(pattern, html); ok {
			title := trimParagraph(decodeEntities(stripTags(m)))
			if title != "" {
				return sanitizeShuba69Title(title)
			}
		}
	}
	return ""
}

func sanitizeShuba69Title(s string) string {
	for _, token := range []string{"_69书吧", "-69书吧", " - 69书吧", "_69shuba", "-69shuba", "最新章节"} {
		s = strings.ReplaceAll(s, token, "")
	}
	return trimParagraph(s)
}

func shuba69Paragraphs(html, title string) []string {
	if body, ok := shuba69TxtnavBody(html); ok {
		return paragraphsFrom(body)
	}

	bodyPatterns := []string{
		`<div[^>]+id=["']chaptercontent["'][^>]*>(.*?)</div>\s*(?:<div[^>]+id=["']bottomlink["']|<div[^>]+class=["']bottom-ad["']|<script)`,
		`<div[^>]+id=["']txt["'][^>]*>(.*?)</div>\s*(?:<div[^>]+class=["']footlink["']|<script)`,
		`<(?:div|section)[^>]+(?:id|class)=["'][^"']*(?:content|chapter|read|text|txt|article)[^"']*["'][^>]*>(.*?)</(?:div|section)>`,
		`<article[^>]*>(.*?)</article>`,
	}
	for _, pattern := range bodyPatterns {
		body, ok := firstMatch(pattern, html)
		if !ok {
			continue
		}
		paragraphs := paragraphsFrom(body)
		if isChapterProse(paragraphs) {
			return paragraphs
		}
	}

	return fallbackProse(html, title)
}

func shuba69TxtnavBody(html string) (string, bool) {
	patterns := []string{
		`<div[^>]+class=["'][^"']*txtnav[^"']*["'][^>]*>(.*?)</div>\s*<div[^>]+class=["'][^"']*page1[^"']*["']`,
		`<div[^>]+class=["'][^"']*txtnav[^"']*["'][^>]*>(.*?)</div>\s*<div[^>]+class=["'][^"']*bottom-ad2[^"']*["']`,
	}
	for _, pattern := range patterns {
		body, ok := firstMatch(pattern, html)
		if !ok || trimParagraph(body) == "" {
			continue
		}
		cleaned := removeMatches(`<h1[^>]*>.*?</h1>`, body)
		cleaned = removeMatches(`<div[^>]+class=["'][^"']*txtinfo[^"']*["'][^>]*>.*?</div>`, cleaned)
		cleaned = removeMatches(`<div[^>]+id=["']txtright["'][^>]*>.*?</div>`, cleaned)
		cleaned = removeMatches(`<div[^>]+class=["'][^"']*contentadv[^"']*["'][^>]*>.*?</div>`, cleaned)
		cleaned = removeMatches(`<div[^>]+class=["'][^"']*bottom-ad[^"']*["'][^>]*>.*?</div>`, cleaned)
		cleaned = removeMatches(`<script.*?</script>`, cleaned)
		if trimParagraph(cleaned) != "" {
			return cleaned, true
		}
	}
	return "", false
}

// fallbackProse reads a chapter from a whole page when no body element was
// found, stopping at the footer's navigation.
func fallbackProse(html, title string) []string {
	page := removeMatches(`<!--.*?-->`, html)
	page = removeMatches(`<script.*?</script>`, page)
	page = removeMatches(`<style.*?</style>`, page)
	page = removeMatches(`<noscript.*?</noscript>`, page)
	page = removeMatches(`<svg.*?</svg>`, page)
	page = replaceMatches(`<br\s*/?>`, page, "\n")
	page = replaceMatches(`</(p|div|section|article|li|h1|h2|h3|tr|td)>`, page, "\n")
	page = collapseNewlines(decodeEntities(stripTags(page)))

	lines := lines(page)
	if len(lines) == 0 {
		return nil
	}

	normalizedTitle := identityHeading(sanitizeShuba69Title(title))
	start := 0
	if normalizedTitle != "" {
		for i, line := range lines {
			if matchesHeading(line, normalizedTitle) {
				start = i + 1
				break
			}
		}
	}
	for start < len(lines) {
		line := lines[start]
		if matchesHeading(line, normalizedTitle) || isMetadataLine(line) {
			start++
			continue
		}
		break
	}

	var collected []string
	for _, line := range lines[min(start, len(lines)):] {
		if isChapterFooterBoundary(line) {
			break
		}
		if isMetadataLine(line) || isNoiseParagraph(line) || matchesHeading(line, normalizedTitle) {
			continue
		}
		if len(collected) == 0 && looksLikeChapterHeading(line) {
			return nil
		}
		collected = append(collected, line)
	}

	if !isChapterProse(collected) || looksLikeNavigationShell(lines, collected) {
		return nil
	}
	return collected
}

// isChapterProse reports whether lines hold enough prose to be a chapter.
func isChapterProse(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	var prose []string
	for _, line := range lines {
		if isLikelyProseLine(line) {
			prose = append(prose, line)
		}
	}
	if len(prose) == 0 {
		return false
	}
	return len(prose) >= 2 || len([]rune(strings.Join(prose, ""))) >= 40
}

// isLikelyProseLine reports whether a line reads as prose rather than a label.
func isLikelyProseLine(line string) bool {
	if len([]rune(line)) >= 12 {
		return true
	}
	if len([]rune(line)) >= 5 {
		for _, r := range "，。！？；：“”‘’" {
			if strings.ContainsRune(line, r) {
				return true
			}
		}
	}
	return false
}

func matchesHeading(line, normalizedTitle string) bool {
	if normalizedTitle == "" {
		return false
	}
	normalizedLine := identityHeading(line)
	if normalizedLine == "" {
		return false
	}
	if normalizedLine == normalizedTitle {
		return true
	}
	shorter := min(len([]rune(normalizedLine)), len([]rune(normalizedTitle)))
	if shorter < 6 {
		return false
	}
	return strings.Contains(normalizedLine, normalizedTitle) || strings.Contains(normalizedTitle, normalizedLine)
}

func isMetadataLine(line string) bool {
	lowered := strings.ToLower(line)
	for _, token := range []string{
		"作者", "更新时间", "字数", "首页", "书页", "收藏", "目录", "设置",
		"正序", "倒序", "加入书签", "返回目录", "朗读", "夜间", "白天", "字体", "护眼",
	} {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return matchesAny(line, `^\d{4}[-/年]\d{1,2}[-/月]\d{1,2}`)
}

func isChapterFooterBoundary(line string) bool {
	lowered := strings.ToLower(line)
	for _, token := range []string{
		"上一章", "下一章", "返回目录", "加入书签", "本章完", "点击下一页", "推荐阅读", "手机阅读",
	} {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return strings.Contains(lowered, "书签") && strings.Contains(lowered, "目录")
}

func looksLikeChapterHeading(line string) bool {
	return matchesAny(line, `^第[\d零〇一二三四五六七八九十百千两\s]+[章节回卷部篇].*`)
}

func looksLikeNavigationShell(all, collected []string) bool {
	headings, breadcrumbs, prose := 0, 0, 0
	for _, line := range all {
		if looksLikeChapterHeading(line) {
			headings++
		}
		if strings.Contains(line, ">") && strings.Contains(line, "首页") {
			breadcrumbs++
		}
	}
	for _, line := range collected {
		if isLikelyProseLine(line) {
			prose++
		}
	}
	return (headings >= 2 && prose <= 1) || (breadcrumbs > 0 && prose <= 1)
}

func matchesAny(s, pattern string) bool {
	m, ok := firstMatch(pattern, s)
	return ok && m != ""
}

// piaotianAdapter reads 飘天文学's catalog and chapter pages.
type piaotianAdapter struct{}

func (piaotianAdapter) ParseCatalog(html string, base *url.URL) (BookCatalog, error) {
	page := decodeEntities(html)
	title := piaotianTitle(page)
	author := piaotianAuthor(page)
	chapters := piaotianChapters(page, base)

	if title == "" {
		return BookCatalog{}, errors.New("飘天文学目录页里没有解析到书名。")
	}
	if len(chapters) == 0 {
		return BookCatalog{}, errors.New("飘天文学目录页里没有找到章节链接，可能站点结构有变化。")
	}

	return BookCatalog{
		Title:     title,
		Author:    orUnknown(author),
		SourceURL: base.String(),
		Chapters:  chapters,
	}, nil
}

func (piaotianAdapter) ParseChapter(html string, page *url.URL) (Chapter, error) {
	doc := decodeEntities(html)
	title := piaotianChapterTitle(doc)
	paragraphs := removeLeadingTitles(piaotianParagraphs(doc, title), title, identityHeading)
	if len(paragraphs) == 0 {
		return Chapter{}, errors.New("章节页 " + page.String() + " 抓到的不是正文页，或没有解析到有效正文。")
	}
	return chapterFrom(title, paragraphs), nil
}

func (piaotianAdapter) SupplementURLs(*url.URL) []*url.URL { return nil }
func (piaotianAdapter) MetadataURLs(*url.URL) []*url.URL   { return nil }

func piaotianTitle(html string) string {
	for _, pattern := range []string{`<title>(.*?)</title>`, `<h1[^>]*>(.*?)</h1>`} {
		m, ok := firstMatch(pattern, html)
		if !ok {
			continue
		}
		title := trimParagraph(decodeEntities(stripTags(m)))
		if title == "" {
			continue
		}
		for _, token := range []string{"无弹窗全文阅读", "最新章节", "_飘天文学", "_PT文学", ",PT文学", "，PT文学"} {
			title = strings.ReplaceAll(title, token, "")
		}
		title = trimParagraph(title)
		if i := strings.IndexAny(title, ",，"); i >= 0 {
			title = trimParagraph(title[:i])
		}
		return title
	}
	return ""
}

func piaotianAuthor(html string) string {
	for _, pattern := range []string{
		`<meta[^>]+name=["']author["'][^>]+content=["']([^"']+)["']`,
		`作者[：: ]*([^<&\n]+)`,
	} {
		if m, ok := firstMatch(pattern, html); ok {
			if author := trimParagraph(decodeEntities(stripTags(m))); author != "" {
				return author
			}
		}
	}
	return ""
}

func piaotianChapters(html string, base *url.URL) []ChapterRef {
	// The catalog is scanned whole rather than inside the 正文卷 wrapper: that
	// wrapper holds nested <div class="list"> blocks, so a non-greedy match on
	// it stops at the first nested </div> and finds no links at all. The
	// chapter filter below is what keeps the site's own navigation out.
	region := html
	matches := allMatches(`<a[^>]+href=["']([^"']+\.html)["'][^>]*>(.*?)</a>`, region)

	seen := map[string]bool{}
	var chapters []ChapterRef
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		href := trimParagraph(m[0])
		title := trimParagraph(decodeEntities(stripTags(m[1])))
		if href == "" || title == "" || strings.EqualFold(href, "index.html") || href == "./" {
			continue
		}
		if !looksLikePiaotianChapter(title) {
			continue
		}
		resolved := resolveURL(base, href)
		if resolved == "" || seen[resolved] {
			continue
		}
		seen[resolved] = true
		chapters = append(chapters, ChapterRef{Title: title, URL: resolved, Order: len(chapters) + 1})
	}
	return chapters
}

func looksLikePiaotianChapter(title string) bool {
	if !strings.Contains(title, "第") || !strings.Contains(title, "章") {
		return false
	}
	for _, token := range []string{"返回目录", "上一章", "下一章", "书页", "书架", "举报", "收藏"} {
		if strings.Contains(title, token) {
			return false
		}
	}
	return true
}

func piaotianChapterTitle(html string) string {
	for _, pattern := range []string{`<h1[^>]*>(.*?)</h1>`, `<title>(.*?)</title>`} {
		m, ok := firstMatch(pattern, html)
		if !ok {
			continue
		}
		title := trimParagraph(decodeEntities(stripTags(m)))
		if title == "" {
			continue
		}
		title = strings.ReplaceAll(title, "PT文学", "")
		title = strings.ReplaceAll(title, "最新章节", "")
		title = trimParagraph(title)
		if i := strings.Index(title, "第"); i >= 0 && strings.Contains(title[i:], "章") {
			return trimParagraph(title[i:])
		}
		if title != "" {
			return title
		}
	}
	return ""
}

func piaotianParagraphs(html, title string) []string {
	bodyPatterns := []string{
		`<div[^>]+class=["']toplink["'][^>]*>.*?</div>(.*?)<div[^>]+class=["']bottomlink["'][^>]*>`,
		`<h1[^>]*>.*?</h1>.*?<div[^>]+class=["']toplink["'][^>]*>.*?</div>(.*?)<div[^>]+class=["']bottomlink["'][^>]*>`,
	}
	for _, pattern := range bodyPatterns {
		body, ok := firstMatch(pattern, html)
		if !ok || trimParagraph(body) == "" {
			continue
		}
		cleaned := removeMatches(`<!--.*?-->`, body)
		cleaned = removeMatches(`<script.*?</script>`, cleaned)
		cleaned = removeMatches(`<table.*?</table>`, cleaned)
		cleaned = removeMatches(`<center.*?</center>`, cleaned)
		cleaned = removeMatches(`<div[^>]+id=["']Commenddiv["'][^>]*>.*?</div>`, cleaned)
		cleaned = removeMatches(`<div[^>]+id=["']feit2["'][^>]*>.*?</div>`, cleaned)
		if trimParagraph(cleaned) == "" {
			continue
		}
		if paragraphs := piaotianParagraphBlocks(cleaned); piaotianProse(paragraphs) {
			return paragraphs
		}
	}

	if body, ok := firstMatch(`<div[^>]+id=["']content["'][^>]*>(.*?)</div>`, html); ok {
		if paragraphs := piaotianParagraphBlocks(body); piaotianProse(paragraphs) {
			return paragraphs
		}
	}

	return piaotianFallback(html, title)
}

func piaotianParagraphBlocks(fragment string) []string {
	normalized := removeMatches(`<!--.*?-->`, fragment)
	normalized = removeMatches(`<script.*?</script>`, normalized)
	normalized = removeMatches(`<style.*?</style>`, normalized)
	normalized = removeMatches(`<noscript.*?</noscript>`, normalized)
	normalized = replaceMatches(`<br\s*/?>\s*<br\s*/?>`, normalized, "\n\n")
	normalized = replaceMatches(`<br\s*/?>`, normalized, "\n")
	normalized = replaceMatches(`</(p|div|section|article|li|h1|h2|h3)>`, normalized, "\n")
	normalized = collapseNewlines(decodeEntities(stripTags(normalized)))

	var out []string
	for _, block := range strings.Split(normalized, "\n\n") {
		block = trimParagraph(block)
		if block == "" || isNoiseParagraph(block) || isMetadataLine(block) {
			continue
		}
		out = append(out, block)
	}
	return out
}

func piaotianProse(paragraphs []string) bool {
	count := 0
	for _, p := range paragraphs {
		if isPiaotianProse(p) {
			count++
		}
	}
	return count >= 2
}

func isPiaotianProse(s string) bool {
	s = trimParagraph(s)
	if s == "" || s == "()" || s == "（）" {
		return false
	}
	for _, token := range []string{"上一章", "下一章", "返回目录", "返回书页", "我的书架", "章节错误", "加入书架", "举报", "PT文学"} {
		if strings.Contains(s, token) {
			return false
		}
	}
	return len([]rune(s)) >= 8
}

func piaotianFallback(html, title string) []string {
	page := removeMatches(`<!--.*?-->`, html)
	page = removeMatches(`<script.*?</script>`, page)
	page = removeMatches(`<style.*?</style>`, page)
	page = removeMatches(`<noscript.*?</noscript>`, page)
	page = replaceMatches(`<br\s*/?>`, page, "\n")
	page = replaceMatches(`</(p|div|section|article|li|h1|h2|h3|tr|td)>`, page, "\n")
	page = collapseNewlines(decodeEntities(stripTags(page)))

	all := lines(page)
	if len(all) == 0 {
		return nil
	}

	normalizedTitle := identityHeading(title)
	var collected []string
	for _, line := range all {
		if isNoiseParagraph(line) || matchesHeading(line, normalizedTitle) {
			continue
		}
		if line == "()" || line == "（ ）" || line == "（）" {
			continue
		}
		if len(collected) > 0 && looksLikePiaotianChapter(line) && strings.Contains(line, "第") {
			break
		}
		collected = append(collected, line)
	}

	if !piaotianProse(collected) {
		return nil
	}
	return collected
}

// sto9Adapter reads 思兔 sto9's catalog and chapter pages, in Simplified
// Chinese.
type sto9Adapter struct{}

func (sto9Adapter) ParseCatalog(html string, base *url.URL) (BookCatalog, error) {
	catalogURL := base
	if normalized := sto9CatalogURL(base); normalized != nil {
		catalogURL = normalized
	}

	title := toSimplified(sto9Title(html))
	author := toSimplified(sto9Author(html))
	chapters := sto9Chapters(html, catalogURL)

	if title == "" {
		return BookCatalog{}, errors.New("思兔 sto9 目录页里没有解析到书名。")
	}
	if len(chapters) == 0 {
		return BookCatalog{}, errors.New("思兔 sto9 目录页里没有找到章节链接，可能站点结构有变化。")
	}
	for i, chapter := range chapters {
		if chapter.Order != i+1 {
			return BookCatalog{}, errors.New("思兔 sto9 完整目录尚未加载：只解析到 " + itoa(len(chapters)) + " 章。请重新同步会话。")
		}
	}

	return BookCatalog{
		Title:     title,
		Author:    orUnknown(author),
		SourceURL: catalogURL.String(),
		Chapters:  chapters,
	}, nil
}

func (sto9Adapter) ParseChapter(html string, page *url.URL) (Chapter, error) {
	title := toSimplified(sto9ChapterTitle(html))
	raw := sto9ChapterParagraphs(html)
	paragraphs := make([]string, 0, len(raw))
	for _, p := range raw {
		paragraphs = append(paragraphs, toSimplified(p))
	}
	paragraphs = removeLeadingTitles(paragraphs, title, identityTitle)

	total := len([]rune(strings.Join(paragraphs, "")))
	if len(paragraphs) == 0 || total < 20 {
		return Chapter{}, errors.New("章节页 " + page.String() + " 抓到的不是正文页，或没有解析到有效正文。")
	}
	return chapterFrom(title, paragraphs), nil
}

func (sto9Adapter) SupplementURLs(base *url.URL) []*url.URL {
	bookID := sto9BookID(base)
	if bookID == "" {
		return nil
	}
	u, err := url.Parse(base.Scheme + "://" + base.Host + "/ajax_novels/chapterlist/" + bookID + ".html")
	if err != nil {
		return nil
	}
	return []*url.URL{u}
}

func (sto9Adapter) MetadataURLs(base *url.URL) []*url.URL {
	bookID := sto9BookID(base)
	if bookID == "" {
		return nil
	}
	u, err := url.Parse(base.Scheme + "://" + base.Host + "/book/" + bookID + ".html")
	if err != nil {
		return nil
	}
	return []*url.URL{u}
}

// sto9BookID reads the book number of a catalog page.
func sto9BookID(u *url.URL) string {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || !strings.EqualFold(parts[0], "book") || !allDigits(parts[1]) || !strings.EqualFold(parts[2], "index.html") {
		return ""
	}
	return parts[1]
}

func sto9Title(html string) string {
	patterns := []string{
		`<meta[^>]+property=["']og:novel:book_name["'][^>]+content=["']([^"']+)["']`,
		`<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:novel:book_name["']`,
		`<div[^>]+class=["'][^"']*booknav2[^"']*["'][^>]*>.*?<h1[^>]*>(.*?)</h1>`,
		`<title[^>]*>(.*?)</title>`,
	}
	for _, pattern := range patterns {
		m, ok := firstMatch(pattern, html)
		if !ok {
			continue
		}
		title := trimParagraph(decodeEntities(stripTags(m)))
		if i := strings.Index(title, "|"); i >= 0 {
			title = title[:i]
		}
		for _, token := range []string{"繁體最新章節列表", "繁体最新章节列表", "最新章節列表", "最新章节列表"} {
			title = strings.ReplaceAll(title, token, "")
		}
		if title = trimParagraph(title); title != "" {
			return title
		}
	}
	return ""
}

func sto9Author(html string) string {
	patterns := []string{
		`<meta[^>]+property=["']og:novel:author["'][^>]+content=["']([^"']+)["']`,
		`<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:novel:author["']`,
		`作者\s*[：:]\s*<a[^>]*>(.*?)</a>`,
		`作者\s*[：:]\s*([^<\r\n]+)`,
	}
	for _, pattern := range patterns {
		if m, ok := firstMatch(pattern, html); ok {
			if author := trimParagraph(decodeEntities(stripTags(m))); author != "" {
				return author
			}
		}
	}
	return ""
}

func sto9Chapters(html string, base *url.URL) []ChapterRef {
	bookID := sto9BookID(base)
	if bookID == "" {
		return nil
	}

	matches := allMatches(`<li[^>]*data-num\s*=\s*["']?(\d+)["']?[^>]*>\s*<a[^>]+href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a>`, html)

	byOrder := map[int]ChapterRef{}
	orderByURL := map[string]int{}
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		order := atoi(m[0])
		if order <= 0 {
			continue
		}
		title := toSimplified(trimParagraph(decodeEntities(stripTags(m[2]))))
		resolved := resolveURL(base, trimParagraph(m[1]))
		if title == "" || resolved == "" || !isSto9ChapterURL(resolved, bookID) {
			continue
		}
		if existing, ok := byOrder[order]; ok && orderByURL[existing.URL] == order {
			delete(orderByURL, existing.URL)
		}
		if previous, ok := orderByURL[resolved]; ok && previous != order {
			delete(byOrder, previous)
		}
		byOrder[order] = ChapterRef{Title: title, URL: resolved, Order: order}
		orderByURL[resolved] = order
	}

	orders := make([]int, 0, len(byOrder))
	for order := range byOrder {
		orders = append(orders, order)
	}
	sort.Ints(orders)

	chapters := make([]ChapterRef, 0, len(orders))
	for _, order := range orders {
		chapters = append(chapters, byOrder[order])
	}
	return chapters
}

func isSto9ChapterURL(rawURL, bookID string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "sto9.com" && !strings.HasSuffix(host, ".sto9.com") {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || !strings.EqualFold(parts[0], "txt") || parts[1] != bookID {
		return false
	}
	chapter := strings.TrimSuffix(strings.ToLower(parts[2]), ".html")
	return allDigits(chapter)
}

func sto9ChapterTitle(html string) string {
	patterns := []string{
		`<div[^>]+class=["'][^"']*txtnav[^"']*["'][^>]*>.*?<h1[^>]*>(.*?)</h1>`,
		`<h1[^>]*>(.*?)</h1>`,
		`<title[^>]*>(.*?)</title>`,
	}
	for _, pattern := range patterns {
		m, ok := firstMatch(pattern, html)
		if !ok {
			continue
		}
		title := trimParagraph(decodeEntities(stripTags(m)))
		if i := strings.Index(title, "|"); i >= 0 {
			title = title[:i]
		}
		if i := strings.LastIndex(title, "_"); i >= 0 {
			if suffix := trimParagraph(title[i+1:]); suffix != "" {
				title = suffix
			}
		}
		if title = trimParagraph(title); title != "" {
			return title
		}
	}
	return ""
}

func sto9ChapterParagraphs(html string) []string {
	body, ok := firstMatch(`<div[^>]+class=["'][^"']*txtnav[^"']*["'][^>]*>(.*?)<div[^>]+class=["'][^"']*page1[^"']*["'][^>]*>`, html)
	if !ok {
		return nil
	}

	normalized := removeMatches(`<h1[^>]*>.*?</h1>`, body)
	normalized = removeMatches(`<div[^>]+class=["'][^"']*(txtright|txtad|txtcenter)[^"']*["'][^>]*>.*?</div>`, normalized)
	normalized = removeMatches(`<script[^>]*>.*?</script>`, normalized)
	normalized = removeMatches(`<style[^>]*>.*?</style>`, normalized)
	normalized = removeMatches(`<noscript[^>]*>.*?</noscript>`, normalized)
	normalized = replaceMatches(`<br\s*/?>`, normalized, "\n")
	normalized = replaceMatches(`</(p|div|section|article)>`, normalized, "\n")
	normalized = collapseNewlines(decodeEntities(stripTags(normalized)))

	var out []string
	for _, line := range strings.Split(normalized, "\n") {
		line = trimParagraph(line)
		if line == "" || isNoiseParagraph(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

// resolveURL resolves href against base and returns its absolute form.
func resolveURL(base *url.URL, href string) string {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func orUnknown(s string) string {
	if s == "" {
		return "Unknown"
	}
	return s
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
