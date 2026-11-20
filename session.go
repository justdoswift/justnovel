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
	Loaded    bool
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
	loaded    bool
	cookies   int
	userAgent string

	// onLoaded runs once each time a page finishes loading, so the app can
	// read the catalog without making the user press a second button.
	onLoaded func()
	// onManual runs when the site demands a human, so the app can bring the
	// browser forward at the one moment it is actually needed.
	onManual func()
	// pendingQuery is a search to submit through the site's own form once the
	// page has loaded, for a site whose search is a POST.
	pendingQuery string
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
			onLoaded := s.onLoaded
			query := s.pendingQuery
			s.pendingQuery = ""
			s.mu.Unlock()
			if !loading || manual {
				return
			}
			s.set("页面已加载。确认内容后同步会话。", false)

			// A site whose search is a POST is searched by filling its own
			// form once the page it lives on has loaded.
			if query != "" {
				s.submitSearchForm(query)
				return
			}

			// A site that blocks the request answers with its own error page,
			// which looks loaded; name it instead of letting the user try to
			// sync a page that holds no catalog.
			go func() {
				text := strings.ToLower(s.eval("document.body ? document.body.innerText.slice(0, 600) : ''"))
				for _, marker := range []string{"page not found", "403 forbidden", "access denied", "just a moment"} {
					if strings.Contains(text, marker) {
						s.set("站点返回了拦截页，请完成验证或更换站点。", false)
						return
					}
				}
				if onLoaded != nil {
					onLoaded()
				}
			}()
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

			lowered := strings.ToLower(e.Title)
			if strings.Contains(lowered, "page not found") || strings.Contains(lowered, "just a moment") {
				s.set("站点返回了拦截页，请完成验证。", false)
			}
		})
	}
	return s
}

func (s *browserSession) State() sessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sessionView{Status: s.status, Title: s.title, LastError: s.lastError,
		Loading: s.loading, Manual: s.manual, Cookies: s.cookies, Loaded: s.loaded}
}

func (s *browserSession) Configure(source Source) {
	s.mu.Lock()
	s.source = source
	s.title = source.SessionTitle
	s.status = "等待打开目录页"
	s.lastError = ""
	s.manual = false
	s.loaded = false
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
	s.loaded = true
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

// OnLoaded sets what runs when a page finishes loading.
func (s *browserSession) OnLoaded(fn func()) {
	s.mu.Lock()
	s.onLoaded = fn
	s.mu.Unlock()
}

// OnManual sets what runs when the site demands a human.
func (s *browserSession) OnManual(fn func()) {
	s.mu.Lock()
	s.onManual = fn
	s.mu.Unlock()
}

// HTML is the page the panel is showing, as the site sent it.
func (s *browserSession) HTML() string {
	return s.eval("document.documentElement.outerHTML")
}

// URL is the address the panel is showing.
func (s *browserSession) URL() string {
	return s.eval("location.href")
}

// SubmitSearchAfterLoad fills the site's own search form and submits it, for a
// site whose search is a POST the panel cannot be pointed at directly.
func (s *browserSession) SubmitSearchAfterLoad(query string) {
	s.mu.Lock()
	s.pendingQuery = query
	s.mu.Unlock()
}

// submitSearchForm types the query into the site's search box and submits the
// form, which is how a POST search has to be started from inside the page.
func (s *browserSession) submitSearchForm(query string) {
	script := fmt.Sprintf(`(function(){
		var q = %q;
		var input = document.querySelector('input[name="searchkey"]');
		if (!input) { return 'no-input'; }
		input.value = q;
		var form = input.form;
		if (form) { form.submit(); return 'submitted'; }
		return 'no-form';
	})()`, query)
	s.eval(script)
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
	s.loaded = html != ""
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
	// The site's complete-catalog endpoint is the fallback: sto9's adapter
	// reads it directly, so a button that never settles must not block the
	// sync.
	s.set("完整目录按钮没有响应，继续读取目录...", true)
	return nil
}

// LoadHTML navigates the panel to a page and returns the HTML the browser
// rendered, waiting out Cloudflare's checks.
func (s *browserSession) LoadHTML(ctx context.Context, page string) (string, error) {
	if s.view == nil {
		return "", DownloadError{"浏览器会话不可用。"}
	}
	s.mu.Lock()
	s.loaded = true
	s.mu.Unlock()

	if sameNavigation(s.eval("location.href"), page) {
		html := s.eval("document.documentElement.outerHTML")
		if html != "" && !looksLikeChallengePage(html) {
			return html, nil
		}
	}

	// Once the browser has passed the site's verification, a same-origin
	// fetch carries its cookies and fingerprint without navigating the
	// panel away from the catalog. It is much faster than loading every
	// chapter as a page, and falls back to a navigation when the site
	// refuses it.
	if html, ok := s.fetchInPage(ctx, page); ok {
		return html, nil
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

// fetchInPage reads a page with the browser's own fetch, so the request
// carries the session's cookies and browser fingerprint without navigating
// the panel. It reports false when the site refuses the fetch or answers
// with a challenge, in which case LoadHTML navigates to the page.
func (s *browserSession) fetchInPage(ctx context.Context, page string) (string, bool) {
	if s.view == nil {
		return "", false
	}
	current := s.eval("location.href")
	if current == "" || current == "about:blank" {
		return "", false
	}

	script := fmt.Sprintf(`try {
		const response = await fetch(%q, {
			credentials: 'include',
			redirect: 'follow',
			headers: {'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8'}
		});
		const text = await response.text();
		return {status: response.status, url: response.url, text: text};
	} catch (error) {
		return {status: 0, url: '', text: '', error: String(error)};
	}`, page)

	value, err := s.view.Page().EvalContext(ctx, script)
	if err != nil {
		return "", false
	}
	result, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	status, _ := result["status"].(float64)
	text, _ := result["text"].(string)
	if status < 200 || status >= 300 || text == "" || looksLikeChallengePage(text) {
		return "", false
	}

	s.mu.Lock()
	s.loading = false
	s.manual = false
	s.status = "浏览器会话已拿到章节内容。"
	s.mu.Unlock()
	s.invalidate()
	return text, true
}

func (s *browserSession) markManualVerification() {
	s.mu.Lock()
	first := !s.notified
	s.manual = true
	s.notified = true
	s.status = "检测到 Cloudflare 验证，请在浏览器面板完成验证。"
	onManual := s.onManual
	s.mu.Unlock()
	s.invalidate()

	// The one moment the browser has to be in front of the user.
	if onManual != nil {
		onManual()
	}

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
