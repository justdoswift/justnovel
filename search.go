package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// SearchHit is one book a site's search turned up.
type SearchHit struct {
	Title  string
	Author string
	URL    string
}

// searchURL builds the page a site searches on, for the query given.
func (s Source) searchURL(query string) string {
	if s.SearchURL == nil {
		return ""
	}
	return s.SearchURL(query)
}

// parseSearch reads a site's search page into books.
func (s Source) parseSearch(html string, base *url.URL) []SearchHit {
	if s.ParseSearch == nil {
		return nil
	}
	return s.ParseSearch(html, base)
}

// searchLink is one anchor of a search page, with its text and href.
type searchLink struct {
	Text string
	Href string
}

// anchors lists the anchors of a page in order, so a parser can walk them.
func anchors(html string) []searchLink {
	pattern := regexp.MustCompile(`(?is)<a\s[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	var out []searchLink
	for _, m := range pattern.FindAllStringSubmatch(html, -1) {
		text := trimParagraph(stripTags(decodeEntities(m[2])))
		if text == "" {
			continue
		}
		out = append(out, searchLink{Text: text, Href: decodeEntities(m[1])})
	}
	return out
}

// absolute resolves an href against the page it came from.
func absolute(base *url.URL, href string) string {
	if base == nil {
		return href
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// dedupeHits drops repeats, keeping the first of each address.
func dedupeHits(hits []SearchHit) []SearchHit {
	seen := map[string]bool{}
	out := hits[:0]
	for _, hit := range hits {
		if hit.URL == "" || seen[hit.URL] {
			continue
		}
		seen[hit.URL] = true
		out = append(out, hit)
	}
	return out
}

// shuba69SearchURL searches 69shuba by title.
func shuba69SearchURL(query string) string {
	return "https://www.69shuba.com/modules/article/search.php?searchkey=" + url.QueryEscape(query)
}

// parseShuba69Search reads 69shuba's results table: a row per book, the title
// linked to /book/<id>/, and the author in the row.
func parseShuba69Search(html string, base *url.URL) []SearchHit {
	page := decodeEntities(html)
	var hits []SearchHit
	rowPattern := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	linkPattern := regexp.MustCompile(`(?is)<a\s[^>]*href=["']([^"']*?/book/\d+/?[^"']*)["'][^>]*>(.*?)</a>`)
	for _, row := range rowPattern.FindAllStringSubmatch(page, -1) {
		m := linkPattern.FindStringSubmatch(row[1])
		if m == nil {
			continue
		}
		title := trimParagraph(stripTags(m[2]))
		if title == "" {
			continue
		}
		hits = append(hits, SearchHit{
			Title:  title,
			Author: shuba69SearchAuthor(row[1]),
			URL:    absolute(base, m[1]),
		})
	}
	return dedupeHits(hits)
}

// shuba69SearchAuthor reads the author cell of a result row.
func shuba69SearchAuthor(row string) string {
	cell := regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	cells := cell.FindAllStringSubmatch(row, -1)
	if len(cells) < 3 {
		return ""
	}
	for _, c := range cells[2:] {
		text := trimParagraph(stripTags(c[1]))
		if text != "" && !allDigits(text) {
			return text
		}
	}
	return ""
}

// piaotianSearchURL searches 飘天文学 by title. The site reads the keyword as
// GBK, so a UTF-8 query matches nothing at all: the bytes have to be encoded
// the way the site's own form encodes them.
func piaotianSearchURL(query string) string {
	return "https://www.piaotia.com/modules/article/search.php?searchkey=" + gbkEscape(query)
}

// gbkEscape percent-encodes a query in GB18030, the encoding the older Chinese
// sites read their forms in.
func gbkEscape(query string) string {
	encoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(query))
	if err != nil {
		return url.QueryEscape(query)
	}
	var out strings.Builder
	for _, b := range encoded {
		out.WriteString("%")
		out.WriteString(strings.ToUpper(fmt.Sprintf("%02x", b)))
	}
	return out.String()
}

// parsePiaotianSearch reads 飘天文学's answer to a search. The site does one of
// two things: it lists every match in a table, or, when the query names one
// book, it redirects straight to that book's page. Both are handled here,
// because the second is what happens for most real titles.
func parsePiaotianSearch(html string, base *url.URL) []SearchHit {
	page := decodeEntities(html)

	if hits := piaotianResultRows(page, base); len(hits) > 0 {
		return hits
	}
	if hit, ok := piaotianBookPage(page, base); ok {
		return []SearchHit{hit}
	}
	return nil
}

// piaotianResultRows reads the results table: each row names the book in a
// /bookinfo/<a>/<b>.html link and the author in its third cell.
func piaotianResultRows(page string, base *url.URL) []SearchHit {
	var hits []SearchHit
	rowPattern := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	bookPattern := regexp.MustCompile(`(?is)<a\s[^>]*href=["']([^"']*?/bookinfo/\d+/\d+\.html[^"']*)["'][^>]*>(.*?)</a>`)
	cellPattern := regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)

	for _, row := range rowPattern.FindAllStringSubmatch(page, -1) {
		m := bookPattern.FindStringSubmatch(row[1])
		if m == nil {
			continue
		}
		title := trimParagraph(stripTags(m[2]))
		if title == "" {
			continue
		}
		hits = append(hits, SearchHit{
			Title:  title,
			Author: piaotianRowAuthor(row[1], cellPattern),
			URL:    absolute(base, m[1]),
		})
	}
	return dedupeHits(hits)
}

// piaotianBookPage reads a book's own page, which is where a search lands when
// it matched exactly one title. The name is the page's h1, the author sits in
// the 作者 cell, and the catalog is the 点击阅读 link.
func piaotianBookPage(page string, base *url.URL) (SearchHit, bool) {
	catalog := regexp.MustCompile(`(?is)<a\s[^>]*href=["']([^"']*?/html/\d+/\d+/?[^"']*)["'][^>]*>`)
	m := catalog.FindStringSubmatch(page)
	if m == nil {
		return SearchHit{}, false
	}

	title := ""
	if h := regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`).FindStringSubmatch(page); h != nil {
		title = trimParagraph(stripTags(h[1]))
	}
	if title == "" {
		if t := regexp.MustCompile(`(?is)<title>(.*?)</title>`).FindStringSubmatch(page); t != nil {
			title = trimParagraph(strings.Split(stripTags(t[1]), "最新章节")[0])
		}
	}
	if title == "" {
		return SearchHit{}, false
	}

	author := ""
	if a := regexp.MustCompile(`(?is)作[^<>]*者[：:]\s*(?:</td>\s*<td[^>]*>)?\s*([^<\s][^<]*)`).FindStringSubmatch(page); a != nil {
		author = trimParagraph(stripTags(a[1]))
	}

	return SearchHit{Title: title, Author: author, URL: absolute(base, m[1])}, true
}

// piaotianRowAuthor reads the author cell of a result row: the third cell of
// the table, after the name and the latest chapter.
func piaotianRowAuthor(row string, cellPattern *regexp.Regexp) string {
	cells := cellPattern.FindAllStringSubmatch(row, -1)
	if len(cells) < 3 {
		return ""
	}
	return trimParagraph(stripTags(cells[2][1]))
}

// sto9Search searches 思兔 by title. The site takes a POST form, so the query
// cannot live in a URL: the search action posts it.
func sto9SearchURL(query string) string {
	return "https://sto9.com/search"
}

// sto9SearchForm is the body the site's own search form posts.
func sto9SearchForm(query string) url.Values {
	return url.Values{"searchkey": {query}, "searchtype": {"all"}}
}

// parseSto9Search reads 思兔's results: each book is an <h3> link to
// /book/<id>.html, with the author in the first label under it.
func parseSto9Search(html string, base *url.URL) []SearchHit {
	page := decodeEntities(html)
	var hits []SearchHit

	itemPattern := regexp.MustCompile(`(?is)<li>(.*?)</li>`)
	titlePattern := regexp.MustCompile(`(?is)<h3[^>]*>\s*<a[^>]*href=["']([^"']*?/book/\d+\.html[^"']*)["'][^>]*>(.*?)</a>`)
	labelPattern := regexp.MustCompile(`(?is)<label[^>]*>(.*?)</label>`)

	for _, item := range itemPattern.FindAllStringSubmatch(page, -1) {
		m := titlePattern.FindStringSubmatch(item[1])
		if m == nil {
			continue
		}
		title := trimParagraph(stripTags(m[2]))
		if title == "" {
			continue
		}
		author := ""
		if labels := labelPattern.FindAllStringSubmatch(item[1], -1); len(labels) > 0 {
			author = trimParagraph(stripTags(labels[0][1]))
		}
		hits = append(hits, SearchHit{
			Title:  title,
			Author: author,
			URL:    absolute(base, m[1]),
		})
	}
	return dedupeHits(hits)
}
