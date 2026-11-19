package main

import (
	"net/url"
	"strings"
	"time"
)

// Source describes a site JustNovel reads books from: how its pages are
// addressed, which session it needs, and how fast it may be requested.
type Source struct {
	Kind        SourceKind
	Name        string
	Placeholder string
	IdleDetail  string
	InvalidText string

	// SessionTitle names the site's session in the status panel.
	SessionTitle string
	// CookieDomains are the cookie domains the session keeps.
	CookieDomains []string
	// NavHosts are the hosts of the site's own pages.
	NavHosts []string
	// RequiresCookies makes an empty cookie jar a session error.
	RequiresCookies bool
	// ExpandButton is the element the webview clicks to load the whole
	// catalog, when the site loads it lazily.
	ExpandButton string
	// PrefersSimplified converts the site's text to Simplified Chinese.
	PrefersSimplified bool

	RateLimitCodes   map[int]bool
	RateLimitDelay   time.Duration
	MaxRateLimitWait int
	// RetryRateLimitForever keeps waiting on a rate limit, for a site that
	// answers 429 until it lets the request through.
	RetryRateLimitForever bool
	ChapterDelayMin       time.Duration
	ChapterDelayMax       time.Duration
}

// Sources lists the sites the app reads, in the order the source menu shows
// them.
var Sources = []Source{
	{
		Kind:             SourceShuba69,
		Name:             "69 书吧",
		Placeholder:      "https://www.69shuba.com/book/90028/",
		IdleDetail:       "输入 69 书吧目录页 URL，然后在右侧浏览器里完成验证。",
		InvalidText:      "请输入有效的 69 书吧目录页链接。",
		SessionTitle:     "69 书吧会话",
		CookieDomains:    []string{"69shuba.com"},
		NavHosts:         []string{"69shuba.com"},
		RequiresCookies:  true,
		RateLimitCodes:   map[int]bool{403: true, 429: true},
		RateLimitDelay:   10 * time.Second,
		MaxRateLimitWait: 10,
		ChapterDelayMin:  1500 * time.Millisecond,
		ChapterDelayMax:  3 * time.Second,
	},
	{
		Kind:                  SourcePiaotian,
		Name:                  "飘天文学",
		Placeholder:           "https://www.piaotia.com/html/15/15709/",
		IdleDetail:            "输入飘天文学目录页 URL，然后在右侧浏览器里完成验证。",
		InvalidText:           "请输入有效的飘天文学目录页链接。",
		SessionTitle:          "飘天文学会话",
		CookieDomains:         []string{"piaotia.com"},
		NavHosts:              []string{"piaotia.com"},
		RequiresCookies:       true,
		RateLimitCodes:        map[int]bool{429: true},
		RateLimitDelay:        time.Second,
		RetryRateLimitForever: true,
	},
	{
		Kind:              SourceSto9,
		Name:              "思兔 sto9",
		Placeholder:       "https://sto9.com/book/13564/index.html",
		IdleDetail:        "输入思兔 sto9 的章节页、书页或目录页 URL。",
		InvalidText:       "请输入有效的思兔 sto9 章节页、书页或目录页链接。",
		SessionTitle:      "思兔 sto9 会话",
		CookieDomains:     []string{"sto9.com"},
		NavHosts:          []string{"sto9.com"},
		ExpandButton:      "loadmore",
		PrefersSimplified: true,
		RateLimitCodes:    map[int]bool{429: true},
		RateLimitDelay:    2 * time.Second,
		MaxRateLimitWait:  10,
		ChapterDelayMin:   300 * time.Millisecond,
		ChapterDelayMax:   700 * time.Millisecond,
	},
}

// SourceFor returns the source of a kind.
func SourceFor(kind SourceKind) Source {
	for _, s := range Sources {
		if s.Kind == kind {
			return s
		}
	}
	return Sources[0]
}

// PersistsCookie reports whether a cookie of the domain belongs to the
// site's session.
func (s Source) PersistsCookie(domain string) bool {
	lowered := strings.ToLower(domain)
	for _, d := range s.CookieDomains {
		if strings.Contains(lowered, d) {
			return true
		}
	}
	return false
}

// OwnPage reports whether the URL is a page of the site.
func (s Source) OwnPage(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	for _, d := range s.NavHosts {
		if strings.Contains(host, d) {
			return true
		}
	}
	return false
}

// CatalogURL validates the URL the user typed and returns the catalog page
// it names, or nil when it names none.
func (s Source) CatalogURL(rawURL string) *url.URL {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	switch s.Kind {
	case SourceShuba69:
		return shuba69CatalogURL(u)
	case SourcePiaotian:
		if piaotianCatalogURL(u) {
			return u
		}
	case SourceSto9:
		return sto9CatalogURL(u)
	}
	return nil
}

// shuba69CatalogURL turns a book page into the catalog page that lists every
// chapter: /book/<id>/ rather than /book/<id>.htm, which holds only the
// latest few.
func shuba69CatalogURL(u *url.URL) *url.URL {
	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "69shuba.com") {
		return nil
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || !strings.EqualFold(parts[0], "book") {
		return nil
	}
	id := strings.TrimSuffix(strings.ToLower(parts[1]), ".htm")
	if !allDigits(id) {
		return nil
	}
	catalog, err := url.Parse(u.Scheme + "://" + u.Host + "/book/" + id + "/")
	if err != nil {
		return nil
	}
	return catalog
}

// piaotianCatalogURL reports whether the URL is /html/<class>/<book>/.
func piaotianCatalogURL(u *url.URL) bool {
	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "piaotia.com") {
		return false
	}
	parts := strings.Split(strings.Trim(strings.ToLower(u.Path), "/"), "/")
	if len(parts) != 3 || parts[0] != "html" {
		return false
	}
	return allDigits(parts[1]) && allDigits(parts[2])
}

// sto9CatalogURL turns a chapter page, book page or catalog page of sto9 into
// the catalog page /book/<id>/index.html.
func sto9CatalogURL(u *url.URL) *url.URL {
	host := strings.ToLower(u.Host)
	if host != "sto9.com" && !strings.HasSuffix(host, ".sto9.com") {
		return nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	var bookID string
	switch {
	case len(parts) == 2 && strings.EqualFold(parts[0], "book"):
		id := strings.TrimSuffix(strings.ToLower(parts[1]), ".html")
		if allDigits(id) {
			bookID = id
		}
	case len(parts) == 3 && strings.EqualFold(parts[0], "book") && allDigits(parts[1]) && strings.EqualFold(parts[2], "index.html"):
		bookID = parts[1]
	case len(parts) == 3 && strings.EqualFold(parts[0], "txt") && allDigits(parts[1]):
		chapter := strings.TrimSuffix(strings.ToLower(parts[2]), ".html")
		if allDigits(chapter) {
			bookID = parts[1]
		}
	}
	if bookID == "" {
		return nil
	}

	catalog, err := url.Parse(u.Scheme + "://" + u.Host + "/book/" + bookID + "/index.html")
	if err != nil {
		return nil
	}
	return catalog
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Adapter parses the pages of one site.
type Adapter interface {
	// ParseCatalog reads the book title, author and chapters.
	ParseCatalog(html string, base *url.URL) (BookCatalog, error)
	// ParseChapter reads one chapter's title and paragraphs.
	ParseChapter(html string, page *url.URL) (Chapter, error)
	// SupplementURLs are pages holding the rest of a lazily loaded catalog.
	SupplementURLs(base *url.URL) []*url.URL
	// MetadataURLs are pages carrying the book's title and author, when the
	// catalog page lacks them.
	MetadataURLs(base *url.URL) []*url.URL
}

// AdapterFor returns the parser of a source.
func AdapterFor(kind SourceKind) Adapter {
	switch kind {
	case SourcePiaotian:
		return piaotianAdapter{}
	case SourceSto9:
		return sto9Adapter{}
	default:
		return shuba69Adapter{}
	}
}
