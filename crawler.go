package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// SessionError means the site asked for a browser session: a Cloudflare
// challenge, or a request it refused. The user solves it in the browser
// panel.
type SessionError struct{ Message string }

func (e SessionError) Error() string { return e.Message }

// DownloadError is a failed request or an unreadable page.
type DownloadError struct{ Message string }

func (e DownloadError) Error() string { return e.Message }

// BrowserLoader loads a page through the app's browser panel, for requests
// the site refuses to the plain HTTP client.
type BrowserLoader func(ctx context.Context, page string) (string, error)

// Crawler fetches the pages of one source, with the session's cookies and
// the source's retry policy.
type Crawler struct {
	source  Source
	adapter Adapter
	client  *http.Client
	agent   string
	referer string
	browser BrowserLoader
}

// NewCrawler builds a crawler of the source.
func NewCrawler(source Source, agent, referer string, cookies []*http.Cookie, browser BrowserLoader) *Crawler {
	jar, _ := cookiejar.New(nil)
	if len(cookies) > 0 {
		byHost := map[string][]*http.Cookie{}
		for _, c := range cookies {
			host := strings.TrimPrefix(c.Domain, ".")
			if host == "" {
				host = strings.TrimPrefix(referer, "https://")
				if i := strings.IndexAny(host, "/:"); i >= 0 {
					host = host[:i]
				}
			}
			byHost[host] = append(byHost[host], c)
		}
		for host, list := range byHost {
			u, err := url.Parse("https://" + host + "/")
			if err != nil {
				continue
			}
			jar.SetCookies(u, list)
		}
	}
	if referer != "" {
		if u, err := url.Parse(referer); err == nil {
			if list, ok := cookiesForHost(jar, u); ok {
				jar.SetCookies(u, list)
			}
		}
	}

	return &Crawler{
		source:  source,
		adapter: AdapterFor(source.Kind),
		agent:   agent,
		referer: referer,
		browser: browser,
		client: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
	}
}

func cookiesForHost(jar http.CookieJar, u *url.URL) ([]*http.Cookie, bool) {
	list := jar.Cookies(u)
	return list, len(list) > 0
}

// FetchCatalog reads the catalog page, preferring the HTML the browser
// panel already shows.
func (c *Crawler) FetchCatalog(ctx context.Context, page string, preferredHTML string) (BookCatalog, error) {
	base, err := url.Parse(page)
	if err != nil {
		return BookCatalog{}, DownloadError{"目录页 URL 无效。"}
	}

	if preferredHTML != "" && !looksLikeChallengePage(preferredHTML) {
		if catalog, err := c.adapter.ParseCatalog(preferredHTML, base); err == nil && len(catalog.Chapters) > 0 {
			return c.withMetadata(ctx, catalog, preferredHTML, base), nil
		}
		if supplements := c.adapter.SupplementURLs(base); len(supplements) > 0 {
			if catalog, err := c.parseWithSupplements(ctx, preferredHTML, base); err == nil {
				return catalog, nil
			}
		}
	}

	html, _, err := c.fetch(ctx, page, true, nil)
	if err != nil {
		return BookCatalog{}, err
	}
	if looksLikeChallengePage(html) {
		return BookCatalog{}, SessionError{"抓取到的还是 Cloudflare 挑战页。请先在浏览器面板里确认正文页面能正常打开，再重试。"}
	}
	return c.parseWithSupplements(ctx, html, base)
}

func (c *Crawler) parseWithSupplements(ctx context.Context, primary string, base *url.URL) (BookCatalog, error) {
	if catalog, err := c.adapter.ParseCatalog(primary, base); err == nil && len(catalog.Chapters) > 0 {
		return c.withMetadata(ctx, catalog, primary, base), nil
	}

	supplements := c.adapter.SupplementURLs(base)
	if len(supplements) == 0 {
		return c.adapter.ParseCatalog(primary, base)
	}

	documents := []string{primary}
	for _, supplement := range supplements {
		html, _, err := c.fetch(ctx, supplement.String(), true, nil)
		if err != nil {
			return BookCatalog{}, err
		}
		if looksLikeChallengePage(html) {
			return BookCatalog{}, SessionError{"完整目录请求仍然返回挑战页。请确认浏览器面板当前能正常打开这个站点后重试。"}
		}
		documents = append(documents, html)
	}
	for _, metadata := range c.adapter.MetadataURLs(base) {
		if html, _, err := c.fetch(ctx, metadata.String(), true, nil); err == nil {
			documents = append(documents, html)
		}
	}

	return c.adapter.ParseCatalog(strings.Join(documents, "\n"), base)
}

func (c *Crawler) withMetadata(ctx context.Context, catalog BookCatalog, primary string, base *url.URL) BookCatalog {
	urls := c.adapter.MetadataURLs(base)
	if len(urls) == 0 {
		return catalog
	}
	documents := []string{primary}
	for _, metadata := range urls {
		if html, _, err := c.fetch(ctx, metadata.String(), true, nil); err == nil {
			documents = append(documents, html)
		}
	}
	if len(documents) == 1 {
		return catalog
	}
	if merged, err := c.adapter.ParseCatalog(strings.Join(documents, "\n"), base); err == nil && len(merged.Chapters) > 0 {
		return merged
	}
	return catalog
}

// FetchChapter reads one chapter, falling back to the browser panel when the
// site refuses the request.
func (c *Crawler) FetchChapter(ctx context.Context, chapter ChapterRef, onTransport func(Transport)) (Chapter, Transport, error) {
	if onTransport == nil {
		onTransport = func(Transport) {}
	}

	html, transport, err := c.fetch(ctx, chapter.URL, false, onTransport)
	if err != nil {
		var sessionErr SessionError
		if errors.As(err, &sessionErr) && c.browser != nil {
			onTransport(TransportBrowser)
			browserHTML, browserErr := c.browser(ctx, chapter.URL)
			if browserErr != nil {
				return Chapter{}, transport, DownloadError{fmt.Sprintf("请求 %s 失败。%s 浏览器回退也失败：%s",
					chapter.URL, sessionErr.Error(), browserErr.Error())}
			}
			html, transport = browserHTML, TransportBrowser
		} else {
			return Chapter{}, transport, chapterError(chapter, err)
		}
	}

	if looksLikeChallengePage(html) {
		return Chapter{}, transport, SessionError{"章节页仍然返回 Cloudflare 挑战页。请确认浏览器面板当前能正常打开这个站点后重试。"}
	}

	content, err := c.adapter.ParseChapter(html, mustParseURL(chapter.URL))
	if err != nil {
		return Chapter{}, transport, chapterError(chapter, err)
	}
	content.Order = chapter.Order
	return content, transport, nil
}

// WaitBetweenChapters waits the source's random delay, so the site is read at
// a human pace.
func (c *Crawler) WaitBetweenChapters(ctx context.Context) error {
	if c.source.ChapterDelayMax <= 0 {
		return nil
	}
	low, high := c.source.ChapterDelayMin, c.source.ChapterDelayMax
	delay := low
	if high > low {
		delay += time.Duration(rand.Int64N(int64(high - low)))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func chapterError(chapter ChapterRef, err error) error {
	return DownloadError{fmt.Sprintf("第 %d 章《%s》加载失败：%s\n%s", chapter.Order, chapter.Title, err.Error(), chapter.URL)}
}

// fetch requests a page with retries, rate-limit waits and the browser
// fallback the catalog and chapter requests differ in.
func (c *Crawler) fetch(ctx context.Context, page string, isCatalog bool, onTransport func(Transport)) (string, Transport, error) {
	rateLimitWaits := 0
	transientRetries := 0

	for {
		if err := ctx.Err(); err != nil {
			return "", TransportSession, err
		}

		html, status, retryAfter, err := c.request(ctx, page)
		if err == nil && status == 0 {
			if looksLikeChallengePage(html) {
				return "", TransportSession, SessionError{"返回了 Cloudflare 挑战页。请先在浏览器面板里完成验证。"}
			}
			return html, TransportSession, nil
		}
		if status != 0 {
			if c.source.RateLimitCodes[status] && !isCatalog {
				rateLimitWaits++
				if !c.source.RetryRateLimitForever && rateLimitWaits > c.source.MaxRateLimitWait {
					return "", TransportSession, DownloadError{fmt.Sprintf("请求 %s 失败。已自动重试，当前仍未恢复：HTTP %d。", page, status)}
				}
				delay := c.source.RateLimitDelay
				if retryAfter > 0 {
					delay = retryAfter
				}
				if err := sleepCtx(ctx, delay); err != nil {
					return "", TransportSession, err
				}
				continue
			}
			if status == http.StatusForbidden {
				return "", TransportSession, SessionError{fmt.Sprintf("请求疑似被站点拦截（HTTP 403）。请确认浏览器面板当前能正常打开该站点后重试。")}
			}
			if status >= 500 && status < 600 && transientRetries < 2 {
				delay := 2 * time.Second
				if transientRetries == 1 {
					delay = 5 * time.Second
				}
				transientRetries++
				if err := sleepCtx(ctx, delay+time.Duration(rand.Int64N(int64(400*time.Millisecond)))); err != nil {
					return "", TransportSession, err
				}
				continue
			}
			return "", TransportSession, DownloadError{fmt.Sprintf("请求 %s 失败：HTTP %d。", page, status)}
		}

		if isTransientNetworkError(err) && transientRetries < 2 {
			delay := 2 * time.Second
			if transientRetries == 1 {
				delay = 5 * time.Second
			}
			transientRetries++
			if err := sleepCtx(ctx, delay); err != nil {
				return "", TransportSession, err
			}
			continue
		}
		return "", TransportSession, DownloadError{err.Error()}
	}
}

// request sends one HTTP request and returns the decoded page, the HTTP
// status, and how long Retry-After asked to wait.
func (c *Crawler) request(ctx context.Context, page string) (string, int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
	if err != nil {
		return "", 0, 0, DownloadError{"URL 无效：" + page}
	}
	agent := c.agent
	if agent == "" {
		agent = "JustNovel"
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh-Hans;q=0.9,en;q=0.6")
	if c.referer != "" {
		req.Header.Set("Referer", c.referer)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", 0, 0, ctx.Err()
		}
		return "", 0, 0, DownloadError{err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", resp.StatusCode, 0, DownloadError{err.Error()}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", resp.StatusCode, retryAfter(resp.Header.Get("Retry-After")), nil
	}
	return decodePage(body), 0, 0, nil
}

// decodePage reads a page in the encoding it declares, falling back from
// UTF-16 with a byte order mark, through UTF-8, to GB18030, the encoding of
// the older Chinese sites.
func decodePage(body []byte) string {
	if len(body) >= 2 && ((body[0] == 0xFF && body[1] == 0xFE) || (body[0] == 0xFE && body[1] == 0xFF)) {
		if converted, _, err := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), body); err == nil {
			return string(converted)
		}
	}
	if validUTF8(body) {
		return string(body)
	}
	if converted, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), body); err == nil {
		return string(converted)
	}
	return string(body)
}

func validUTF8(b []byte) bool {
	return strings.ToValidUTF8(string(b), "\uFFFD") == string(b)
}

func retryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

func isTransientNetworkError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, token := range []string{
		"timeout", "connection reset", "connection refused", "broken pipe",
		"network is unreachable", "no such host", "eof", "tls handshake",
	} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// looksLikeChallengePage reports whether the page is a Cloudflare challenge.
func looksLikeChallengePage(html string) bool {
	lowered := strings.ToLower(html)
	return strings.Contains(lowered, "just a moment") ||
		strings.Contains(lowered, "cf_chl_opt") ||
		strings.Contains(lowered, "enable javascript and cookies to continue")
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		return &url.URL{}
	}
	return u
}
