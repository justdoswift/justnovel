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
	Example     string
	IdleDetail  string
	InvalidText string

	// SessionTitle names the site's session in the status panel.
	SessionTitle string
	// CookieDomains are the cookie domains the session keeps.
	CookieDomains []string
	// NavHosts are the hosts of the site's own pages.
	NavHosts []string
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

	// SearchURL builds the page that searches this site for a title. The
	// search runs in the browser panel, because the sites either sit behind
	// Cloudflare, take a POST form, or fill their results with script.
	SearchURL func(query string) string
	// ParseSearch reads the search page's links into books.
	ParseSearch func(html string, base *url.URL) []SearchHit
}

// Sources lists the sites the app reads, in the order the source menu shows
// them.
var Sources = []Source{
	{
		Kind:             SourceShuba69,
		Name:             "69 书吧",
		Placeholder:      "粘贴目录页或章节页地址",
		Example:          "https://www.69shuba.com/book/90028/",
		IdleDetail:       "粘贴 69 书吧的书籍页、目录页或章节页，回车打开目录。",
		InvalidText:      "请输入 69 书吧的书籍页、目录页或章节页链接。",
		SessionTitle:     "69 书吧会话",
		CookieDomains:    []string{"69shuba.com"},
		NavHosts:         []string{"69shuba.com"},
		RateLimitCodes:   map[int]bool{429: true},
		RateLimitDelay:   10 * time.Second,
		MaxRateLimitWait: 10,
		ChapterDelayMin:  1500 * time.Millisecond,
		ChapterDelayMax:  3 * time.Second,
		SearchURL:        shuba69SearchURL,
		ParseSearch:      parseShuba69Search,
	},
	{
		Kind:                  SourcePiaotian,
		Name:                  "飘天文学",
		Placeholder:           "粘贴目录页或章节页地址",
		Example:               "https://www.piaotia.com/html/15/15709/",
		IdleDetail:            "粘贴飘天文学的目录页或章节页，回车打开目录。",
		InvalidText:           "请输入飘天文学的目录页或章节页链接。",
		SessionTitle:          "飘天文学会话",
		CookieDomains:         []string{"piaotia.com"},
		NavHosts:              []string{"piaotia.com"},
		RateLimitCodes:        map[int]bool{429: true},
		RateLimitDelay:        time.Second,
		RetryRateLimitForever: true,
		ChapterDelayMin:       400 * time.Millisecond,
		ChapterDelayMax:       900 * time.Millisecond,
		SearchURL:             piaotianSearchURL,
		ParseSearch:           parsePiaotianSearch,
	},
	{
		Kind:              SourceSto9,
		Name:              "思兔 sto9",
		Placeholder:       "粘贴目录页或章节页地址",
		Example:           "https://sto9.com/book/13564/index.html",
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
		SearchURL:         sto9SearchURL,
		ParseSearch:       parseSto9Search,
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
	raw := strings.TrimSpace(rawURL)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	switch s.Kind {
	case SourceShuba69:
		return shuba69CatalogURL(u)
	case SourcePiaotian:
		return piaotianCatalogURL(u)
	case SourceSto9:
		return sto9CatalogURL(u)
	}
	return nil
}

// shuba69CatalogURL turns a book page, catalog page or chapter page into the
// catalog page that lists every chapter: /book/<id>/ rather than
// /book/<id>.htm, which holds only the latest few.
func shuba69CatalogURL(u *url.URL) *url.URL {
	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "69shuba.com") {
		return nil
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	var id string
	switch {
	case len(parts) >= 2 && strings.EqualFold(parts[0], "book"):
		id = strings.TrimSuffix(strings.ToLower(parts[1]), ".htm")
	case len(parts) >= 3 && strings.EqualFold(parts[0], "txt"):
		id = parts[1]
	}
	if !allDigits(id) {
		return nil
	}
	catalog, err := url.Parse(u.Scheme + "://" + u.Host + "/book/" + id + "/")
	if err != nil {
		return nil
	}
	return catalog
}

// piaotianCatalogURL turns a catalog page or chapter page into the catalog
// page /html/<class>/<book>/.
func piaotianCatalogURL(u *url.URL) *url.URL {
	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "piaotia.com") {
		return nil
	}
	parts := strings.Split(strings.Trim(strings.ToLower(u.Path), "/"), "/")
	// A book page is /bookinfo/<a>/<b>.html, a catalog is /html/<a>/<b>/, and
	// a chapter is /html/<a>/<b>/<n>.html. All three name the same book.
	if len(parts) == 3 && parts[0] == "bookinfo" {
		parts[2] = strings.TrimSuffix(parts[2], ".html")
		if !allDigits(parts[1]) || !allDigits(parts[2]) {
			return nil
		}
		catalog, err := url.Parse(u.Scheme + "://" + u.Host + "/html/" + parts[1] + "/" + parts[2] + "/")
		if err != nil {
			return nil
		}
		return catalog
	}
	if len(parts) != 3 && len(parts) != 4 {
		return nil
	}
	if parts[0] != "html" || !allDigits(parts[1]) || !allDigits(parts[2]) {
		return nil
	}
	if len(parts) == 4 {
		chapter := strings.TrimSuffix(parts[3], ".html")
		if !allDigits(chapter) {
			return nil
		}
	}
	catalog, err := url.Parse(u.Scheme + "://" + u.Host + "/html/" + parts[1] + "/" + parts[2] + "/")
	if err != nil {
		return nil
	}
	return catalog
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
