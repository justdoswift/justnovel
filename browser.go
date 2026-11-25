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

// PanelState is what the view reads of the browser panel.
type PanelState struct {
	Status    string
	Title     string
	LastError string
	Loading   bool
	Manual    bool
	Cookies   int
	Loaded    bool
}

// browserPanel is the site's browser: it holds the page the user verifies in,
// hands the app its cookies, and fetches pages the plain HTTP client may not.
//
// The page is the only way through a Cloudflare check, so it has to be in the
// window and visible at the moment the site asks for a human. It stays in the
// tree at all times (hidden when not needed) because a web view that is not
// placed never loads anything.
type browserPanel struct {
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

	// onLoaded runs once each time a page finishes loading. The app sets it
	// per operation and clears it when the operation ends, so a page that
	// settles late cannot start work nobody asked for.
	onLoaded func()
	// onManual runs when the site demands a human.
	onManual func()
	// pendingQuery is a search to submit through the site's own form once the
	// page has loaded, for a site whose search is a POST.
	pendingQuery string
}

// newBrowserPanel builds the panel for a window and its web view.
func newBrowserPanel(win *mygo.Window, view *mygo.WebView, source Source) *browserPanel {
	p := &browserPanel{
		win:       win,
		view:      view,
		source:    source,
		status:    "等待打开页面",
		title:     source.SessionTitle,
		userAgent: "JustNovel",
	}
	if view == nil {
		return p
	}

	view.Page().OnDidFinishLoad(func() {
		p.mu.Lock()
		loading, manual := p.loading, p.manual
		onLoaded := p.onLoaded
		query := p.pendingQuery
		p.pendingQuery = ""
		p.mu.Unlock()
		if !loading || manual {
			return
		}
		p.set("页面已加载。", false)

		// A site whose search is a POST is searched by filling its own form
		// once the page it lives on has loaded.
		if query != "" {
			p.submitSearchForm(query)
			return
		}

		// A site that blocks the request answers with its own error page,
		// which looks loaded; name it instead of letting the app read a page
		// that holds no catalog.
		go func() {
			text := strings.ToLower(p.eval("document.body ? document.body.innerText.slice(0, 600) : ''"))
			for _, marker := range []string{"page not found", "403 forbidden", "access denied", "just a moment"} {
				if strings.Contains(text, marker) {
					p.set("站点返回了拦截页，请完成验证或更换站点。", false)
					return
				}
			}
			if onLoaded != nil {
				onLoaded()
			}
		}()
	})
	view.Page().OnDidFailLoad(func(err *mygo.LoadError) {
		p.mu.Lock()
		loading := p.loading
		p.lastError = err.Description
		p.mu.Unlock()
		if loading {
			p.set("页面加载失败，请重试。", false)
		}
	})
	view.Page().OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		p.mu.Lock()
		p.title = e.Title
		p.mu.Unlock()

		lowered := strings.ToLower(e.Title)
		if strings.Contains(lowered, "page not found") || strings.Contains(lowered, "just a moment") {
			p.set("站点返回了拦截页，请完成验证。", false)
		}
	})
	return p
}

// State is what the view shows of the panel.
func (p *browserPanel) State() PanelState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PanelState{Status: p.status, Title: p.title, LastError: p.lastError,
		Loading: p.loading, Manual: p.manual, Cookies: p.cookies, Loaded: p.loaded}
}

// Configure points the panel at a site, clearing its page state.
func (p *browserPanel) Configure(source Source) {
	p.mu.Lock()
	p.source = source
	p.title = source.SessionTitle
	p.status = "等待打开页面"
	p.lastError = ""
	p.manual = false
	p.loaded = false
	p.onLoaded = nil
	p.pendingQuery = ""
	p.mu.Unlock()
	p.invalidate()
}

func (p *browserPanel) set(status string, loading bool) {
	p.mu.Lock()
	p.status = status
	p.loading = loading
	p.mu.Unlock()
	p.invalidate()
}

func (p *browserPanel) invalidate() {
	if p.win != nil {
		p.win.Invalidate()
	}
}

// Open shows a page in the panel, where the user solves the site's
// verification.
func (p *browserPanel) Open(page string) {
	p.mu.Lock()
	p.lastError = ""
	p.manual = false
	p.notified = false
	p.loaded = true
	p.status = "正在打开页面，请确认内容正常显示。"
	p.loading = true
	p.mu.Unlock()

	if p.view != nil {
		_ = p.view.Page().LoadURL(page)
	}
	p.invalidate()
}

// OnLoaded sets what runs when a page finishes loading.
func (p *browserPanel) OnLoaded(fn func()) {
	p.mu.Lock()
	p.onLoaded = fn
	p.mu.Unlock()
}

// ClearLoaded drops the load handler, so a page that settles after the
// operation ended starts nothing.
func (p *browserPanel) ClearLoaded() {
	p.OnLoaded(nil)
}

// OnManual sets what runs when the site demands a human.
func (p *browserPanel) OnManual(fn func()) {
	p.mu.Lock()
	p.onManual = fn
	p.mu.Unlock()
}

// HTML is the page the panel is showing, as the site sent it.
func (p *browserPanel) HTML() string {
	return p.eval("document.documentElement.outerHTML")
}

// URL is the address the panel is showing.
func (p *browserPanel) URL() string {
	return p.eval("location.href")
}

// SubmitSearchAfterLoad fills the site's own search form and submits it, for a
// site whose search is a POST the panel cannot be pointed at directly.
func (p *browserPanel) SubmitSearchAfterLoad(query string) {
	p.mu.Lock()
	p.pendingQuery = query
	p.mu.Unlock()
}

// submitSearchForm types the query into the site's search box and submits the
// form, which is how a POST search has to be started from inside the page.
func (p *browserPanel) submitSearchForm(query string) {
	script := fmt.Sprintf(`(function(){
		var q = %q;
		var input = document.querySelector('input[name="searchkey"]');
		if (!input) { return 'no-input'; }
		input.value = q;
		var form = input.form;
		if (form) { form.submit(); return 'submitted'; }
		return 'no-form';
	})()`, query)
	p.eval(script)
}

func (p *browserPanel) eval(script string) string {
	if p.view == nil {
		return ""
	}
	value, err := p.view.Page().Eval(script)
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
func (p *browserPanel) Sync(ctx context.Context) (html string, cookies []*http.Cookie, agent string, err error) {
	p.set("正在同步会话...", true)

	if err := p.expandCatalog(ctx); err != nil {
		return "", nil, "", err
	}

	html = p.eval("document.documentElement.outerHTML")
	cookies = p.Cookies()
	agent = p.eval("navigator.userAgent")
	if agent == "" {
		agent = "JustNovel"
	}

	title := p.eval("document.title")
	if title == "" {
		title = p.source.SessionTitle
	}

	p.mu.Lock()
	p.userAgent = agent
	p.cookies = len(cookies)
	p.loading = false
	p.loaded = html != ""
	p.title = title
	if len(cookies) == 0 {
		p.status = "页面会话已同步。"
	} else {
		p.status = fmt.Sprintf("已同步 %d 个 Cookie。", len(cookies))
	}
	p.manual = false
	p.notified = false
	p.mu.Unlock()
	p.invalidate()

	return html, cookies, agent, nil
}

// Cookies reads the site's cookies from the app's cookie store, HTTPOnly ones
// included.
func (p *browserPanel) Cookies() []*http.Cookie {
	var out []*http.Cookie
	seen := map[string]bool{}
	for _, domain := range p.source.CookieDomains {
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
func (p *browserPanel) Agent() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.userAgent == "" {
		return "JustNovel"
	}
	return p.userAgent
}

// expandCatalog clicks the site's button that loads the whole chapter list,
// and waits for it to finish.
func (p *browserPanel) expandCatalog(ctx context.Context) error {
	button := p.source.ExpandButton
	if button == "" || p.view == nil {
		return nil
	}

	script := fmt.Sprintf(`(() => {
		const button = document.getElementById(%q);
		if (!button) return 'ready';
		if (!(button.textContent || '').includes('正在')) button.click();
		return 'waiting';
	})()`, button)
	if p.eval(script) == "ready" {
		return nil
	}

	p.set("正在加载完整目录...", true)
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		if p.eval(fmt.Sprintf(`document.getElementById(%q) ? 'waiting' : 'ready'`, button)) == "ready" {
			return nil
		}
	}
	// The site's complete-catalog endpoint is the fallback: sto9's adapter
	// reads it directly, so a button that never settles must not block the
	// sync.
	p.set("完整目录按钮没有响应，继续读取目录...", true)
	return nil
}

// LoadHTML navigates the panel to a page and returns the HTML the browser
// rendered, waiting out Cloudflare's checks. It is the crawler's browser
// fallback.
func (p *browserPanel) LoadHTML(ctx context.Context, page string) (string, error) {
	if p.view == nil {
		return "", DownloadError{"浏览器会话不可用。"}
	}
	p.mu.Lock()
	p.loaded = true
	p.mu.Unlock()

	if sameNavigation(p.eval("location.href"), page) {
		html := p.eval("document.documentElement.outerHTML")
		if html != "" && !looksLikeChallengePage(html) {
			return html, nil
		}
	}

	// Once the browser has passed the site's verification, a same-origin
	// fetch carries its cookies and fingerprint without navigating the panel
	// away from the catalog. It is much faster than loading every chapter as
	// a page, and falls back to a navigation when the site refuses it.
	if html, ok := p.fetchInPage(ctx, page); ok {
		return html, nil
	}

	p.set("正在通过浏览器会话加载 "+fileNameOf(page)+"...", true)
	if err := p.view.Page().LoadURL(page); err != nil {
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

		html := p.eval("document.documentElement.outerHTML")
		current := p.eval("location.href")
		if html == "" {
			continue
		}
		if looksLikeChallengePage(html) {
			challenges++
			p.set("站点正在完成 Cloudflare 验证，等待自动跳转...", true)
			if challenges >= 3 {
				p.markManualVerification()
			}
			continue
		}
		ready := p.eval("document.readyState")
		if ready != "complete" && ready != "interactive" {
			continue
		}
		if current != "" && p.source.OwnPage(current) && !sameNavigation(current, page) {
			continue
		}
		p.mu.Lock()
		p.loading = false
		p.manual = false
		p.status = "浏览器会话已拿到章节内容。"
		p.mu.Unlock()
		p.invalidate()
		return html, nil
	}

	return "", SessionError{"浏览器会话仍停留在 Cloudflare 挑战页。请先在浏览器面板完成验证后重试。"}
}

// fetchInPage reads a page with the browser's own fetch, so the request
// carries the session's cookies and browser fingerprint without navigating
// the panel. It reports false when the site refuses the fetch or answers
// with a challenge, in which case LoadHTML navigates to the page.
func (p *browserPanel) fetchInPage(ctx context.Context, page string) (string, bool) {
	if p.view == nil {
		return "", false
	}
	current := p.eval("location.href")
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

	value, err := p.view.Page().EvalContext(ctx, script)
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

	p.mu.Lock()
	p.loading = false
	p.manual = false
	p.status = "浏览器会话已拿到章节内容。"
	p.mu.Unlock()
	p.invalidate()
	return text, true
}

func (p *browserPanel) markManualVerification() {
	p.mu.Lock()
	first := !p.notified
	p.manual = true
	p.notified = true
	p.status = "检测到 Cloudflare 验证，请在浏览器面板完成验证。"
	onManual := p.onManual
	p.mu.Unlock()
	p.invalidate()

	// The one moment the browser has to be in front of the user.
	if onManual != nil {
		onManual()
	}

	if first && mygo.NotificationsSupported() {
		note := mygo.NewNotification(mygo.NotificationOptions{
			Title: "需要完成 Cloudflare 验证",
			Body:  "检测到站点触发了 Cloudflare 验证，请在浏览器面板中完成人工验证，然后重试。",
		})
		go func() { _ = note.Show() }()
	}
}

// WebView is the native view the window places.
func (p *browserPanel) WebView() *mygo.WebView { return p.view }

// sameNavigation compares two URLs by host and path, ignoring the query and
// the fragment.
func sameNavigation(a, b string) bool {
	ua, ub := mustParseURL(a), mustParseURL(b)
	if ua.Host == "" || ub.Host == "" {
		return false
	}
	return strings.EqualFold(ua.Host, ub.Host) && strings.TrimSuffix(ua.Path, "/") == strings.TrimSuffix(ub.Path, "/")
}

// fileNameOf is the last path segment of a page, for the panel's status line.
func fileNameOf(page string) string {
	u := mustParseURL(page)
	if u.Path == "" {
		return u.Host
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return parts[len(parts)-1]
}
