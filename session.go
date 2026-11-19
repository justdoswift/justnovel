package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// sessionView is what the view reads of the browser panel.
type sessionView struct {
	Status    string
	Title     string
	LastError string
	Loading   bool
	Manual    bool
	Cookies   int
}

// browserSession is the site's browser panel: it holds the page the user
// verifies in, hands the app its cookies, and fetches pages the plain HTTP
// client may not.
type browserSession struct {
	mu        sync.Mutex
	win       *mygo.Window
	view      *mygo.WebView
	source    Source
	status    string
	title     string
	lastError string
	loading   bool
	manual    bool
	notified  bool
	cookies   int
	userAgent string
}

func newBrowserSession(win *mygo.Window, view *mygo.WebView, source Source) *browserSession {
	s := &browserSession{
		win:       win,
		view:      view,
		source:    source,
		status:    "等待打开目录页",
		title:     source.SessionTitle,
		userAgent: "JustNovel",
	}
	if view != nil {
		view.Page().OnDidFinishLoad(func() {
			s.mu.Lock()
			loading, manual := s.loading, s.manual
			s.mu.Unlock()
			if loading && !manual {
				s.set("页面已加载。确认内容后同步会话。", false)
			}
		})
		view.Page().OnDidFailLoad(func(err *mygo.LoadError) {
			s.mu.Lock()
			loading := s.loading
			s.lastError = err.Description
			s.mu.Unlock()
			if loading {
				s.set("页面加载失败，请重试。", false)
			}
		})
		view.Page().OnPageTitleUpdated(func(e *mygo.TitleEvent) {
			s.mu.Lock()
			s.title = e.Title
			s.mu.Unlock()
		})
	}
	return s
}

func (s *browserSession) State() sessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sessionView{Status: s.status, Title: s.title, LastError: s.lastError, Loading: s.loading, Manual: s.manual, Cookies: s.cookies}
}

func (s *browserSession) Configure(source Source) {
	s.mu.Lock()
	s.source = source
	s.title = source.SessionTitle
	s.status = "等待打开目录页"
	s.lastError = ""
	s.manual = false
	s.mu.Unlock()
}

func (s *browserSession) set(status string, loading bool) {
	s.mu.Lock()
	s.status = status
	s.loading = loading
	s.mu.Unlock()
	s.invalidate()
}

func (s *browserSession) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// Open shows the page in the panel, where the user solves the site's
// verification.
func (s *browserSession) Open(page string) {
	s.mu.Lock()
	s.lastError = ""
	s.manual = false
	s.notified = false
	s.status = "正在打开目录页，请确认页面正常显示。"
	s.loading = true
	s.mu.Unlock()

	if s.view != nil {
		_ = s.view.Page().LoadURL(page)
	}
	s.invalidate()
}

// ResetManual clears the verification notice.
func (s *browserSession) ResetManual() {
	s.mu.Lock()
	s.manual = false
	s.notified = false
	s.mu.Unlock()
}

func (s *browserSession) eval(script string) string {
	if s.view == nil {
		return ""
	}
	value, err := s.view.Page().Eval(script)
	if err != nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// Sync reads the panel's cookies and page, after loading a lazily built
// catalog if the site has one.
func (s *browserSession) Sync(ctx context.Context) (html string, cookies []*http.Cookie, agent string, err error) {
	s.set("正在同步会话...", true)

	if err := s.expandCatalog(ctx); err != nil {
		return "", nil, "", err
	}

	html = s.eval("document.documentElement.outerHTML")
	cookies = s.Cookies()
	if len(cookies) == 0 && s.source.RequiresCookies {
		return "", nil, "", SessionError{"还没有拿到站点 Cookie。请先在浏览器面板完成验证，再点击同步会话。"}
	}
	agent = s.eval("navigator.userAgent")
	if agent == "" {
		agent = "JustNovel"
	}

	title := s.eval("document.title")
	if title == "" {
		title = s.source.SessionTitle
	}

	s.mu.Lock()
	s.userAgent = agent
	s.cookies = len(cookies)
	s.loading = false
	s.title = title
	if len(cookies) == 0 {
		s.status = "页面会话已同步。"
	} else {
		s.status = fmt.Sprintf("已同步 %d 个 Cookie。", len(cookies))
	}
	s.manual = false
	s.notified = false
	s.mu.Unlock()
	s.invalidate()

	return html, cookies, agent, nil
}

// Cookies reads the site's cookies from the app's cookie store, HTTPOnly
// ones included.
func (s *browserSession) Cookies() []*http.Cookie {
	var out []*http.Cookie
	seen := map[string]bool{}
	for _, domain := range s.source.CookieDomains {
		list, err := mygo.Cookies.List(mygo.CookieFilter{Domain: domain})
		if err != nil {
			continue
		}
		for _, c := range list {
			key := c.Name + "|" + c.Domain + "|" + c.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			cookie := &http.Cookie{
				Name:     c.Name,
				Value:    c.Value,
				Path:     c.Path,
				Domain:   c.Domain,
				Secure:   c.Secure,
				HttpOnly: c.HTTPOnly,
			}
			if !c.Expires.IsZero() {
				cookie.Expires = c.Expires
			}
			out = append(out, cookie)
		}
	}
	return out
}

// Agent is the browser's user agent, for the HTTP client's requests.
func (s *browserSession) Agent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userAgent == "" {
		return "JustNovel"
	}
	return s.userAgent
}

// expandCatalog clicks the site's button that loads the whole chapter list,
// and waits for it to finish.
func (s *browserSession) expandCatalog(ctx context.Context) error {
	button := s.source.ExpandButton
	if button == "" || s.view == nil {
		return nil
	}

	script := fmt.Sprintf(`(() => {
		const button = document.getElementById(%q);
		if (!button) return 'ready';
		if (!(button.textContent || '').includes('正在')) button.click();
		return 'waiting';
	})()`, button)
	if s.eval(script) == "ready" {
		return nil
	}

	s.set("正在加载完整目录...", true)
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		if s.eval(fmt.Sprintf(`document.getElementById(%q) ? 'waiting' : 'ready'`, button)) == "ready" {
			return nil
		}
	}
	return DownloadError{"完整目录加载超时。请确认网络正常后重新同步会话。"}
}

// LoadHTML navigates the panel to a page and returns the HTML the browser
// rendered, waiting out Cloudflare's checks.
func (s *browserSession) LoadHTML(ctx context.Context, page string) (string, error) {
	if s.view == nil {
		return "", DownloadError{"浏览器会话不可用。"}
	}

	if sameNavigation(s.eval("location.href"), page) {
		html := s.eval("document.documentElement.outerHTML")
		if html != "" && !looksLikeChallengePage(html) {
			return html, nil
		}
	}

	s.set("正在通过浏览器会话加载 "+fileNameOf(page)+"...", true)
	if err := s.view.Page().LoadURL(page); err != nil {
		return "", DownloadError{"浏览器会话没有成功发起加载。"}
	}

	deadline := time.Now().Add(45 * time.Second)
	challenges := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}

		html := s.eval("document.documentElement.outerHTML")
		current := s.eval("location.href")
		if html == "" {
			continue
		}
		if looksLikeChallengePage(html) {
			challenges++
			s.set("站点正在完成 Cloudflare 验证，等待自动跳转...", true)
			if challenges >= 3 {
				s.markManualVerification()
			}
			continue
		}
		ready := s.eval("document.readyState")
		if ready != "complete" && ready != "interactive" {
			continue
		}
		if current != "" && s.source.OwnPage(current) && !sameNavigation(current, page) {
			continue
		}
		s.mu.Lock()
		s.loading = false
		s.manual = false
		s.status = "浏览器会话已拿到章节内容。"
		s.mu.Unlock()
		s.invalidate()
		return html, nil
	}

	return "", SessionError{"浏览器会话仍停留在 Cloudflare 挑战页。请先在浏览器面板完成验证后重试。"}
}

func (s *browserSession) markManualVerification() {
	s.mu.Lock()
	first := !s.notified
	s.manual = true
	s.notified = true
	s.status = "检测到 Cloudflare 验证，请在浏览器面板完成验证。"
	s.mu.Unlock()
	s.invalidate()

	if first && mygo.NotificationsSupported() {
		note := mygo.NewNotification(mygo.NotificationOptions{
			Title: "需要完成 Cloudflare 验证",
			Body:  "检测到站点触发了 Cloudflare 验证，请在浏览器面板中完成人工验证，然后重新同步会话或继续下载。",
		})
		go func() { _ = note.Show() }()
	}
}

// sameNavigation compares two URLs by host and path, ignoring the query and
// the fragment.
func sameNavigation(a, b string) bool {
	ua, ub := mustParseURL(a), mustParseURL(b)
	if ua.Host == "" || ub.Host == "" {
		return false
	}
	return strings.EqualFold(ua.Host, ub.Host) && strings.TrimSuffix(ua.Path, "/") == strings.TrimSuffix(ub.Path, "/")
}

func fileNameOf(page string) string {
	u := mustParseURL(page)
	if u.Path == "" {
		return u.Host
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return parts[len(parts)-1]
}
