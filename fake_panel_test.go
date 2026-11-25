package main

import (
	"context"
	"net/http"
	"sync"
)

// fakePage is a scripted stand-in for the browser panel's page: it hands the
// app whatever HTML a test sets, and reports the load event when the test says
// the page settled.
//
// It exists so the catalog flow can be tested end to end without a real web
// view, which is the only way to keep the regression for the deadlock honest.
type fakePage struct {
	mu      sync.Mutex
	html    string
	url     string
	loaded  func()
	manual  func()
	queries []string
}

// fakePanel is a browserPanel whose page is scripted.
type fakePanel struct {
	*browserPanel
	page *fakePage
}

// newFakePanel builds a panel over a scripted page.
func newFakePanel(page *fakePage, source Source) *fakePanel {
	p := &browserPanel{source: source, status: "等待打开页面", title: source.SessionTitle}
	return &fakePanel{browserPanel: p, page: page}
}

// OnLoaded records the load handler, and points the scripted page's load
// event at it, which is what a real page does.
func (p *fakePanel) OnLoaded(fn func()) {
	p.mu.Lock()
	p.onLoaded = fn
	p.mu.Unlock()
	p.page.setLoaded(fn)
}

// ClearLoaded drops the load handler.
func (p *fakePanel) ClearLoaded() { p.OnLoaded(nil) }

// OnManual records the manual handler.
func (p *fakePanel) OnManual(fn func()) {
	p.mu.Lock()
	p.onManual = fn
	p.mu.Unlock()
}

// Open records the page and marks the panel as loading.
func (p *fakePanel) Open(page string) {
	p.mu.Lock()
	p.loading = true
	p.loaded = true
	p.status = "正在打开页面。"
	p.mu.Unlock()
	p.page.setURL(page)
}

// HTML is the page the panel would show.
func (p *fakePanel) HTML() string { return p.page.HTML() }

// URL is the address the panel would show.
func (p *fakePanel) URL() string { return p.page.URL() }

// SubmitSearchAfterLoad records a POST search.
func (p *fakePanel) SubmitSearchAfterLoad(query string) {
	p.mu.Lock()
	p.pendingQuery = query
	p.mu.Unlock()
}

// Sync hands the app the scripted page, as a settled browser would.
func (p *fakePanel) Sync(ctx context.Context) (string, []*http.Cookie, string, error) {
	p.mu.Lock()
	p.loading = false
	p.loaded = true
	p.mu.Unlock()
	return p.page.HTML(), nil, "JustNovel test", nil
}

// Agent is the scripted user agent.
func (p *fakePanel) Agent() string { return "JustNovel test" }

// Cookies is empty: the scripted page needs no session.
func (p *fakePanel) Cookies() []*http.Cookie { return nil }

// LoadHTML is the crawler's browser fallback, which the scripted page serves.
func (p *fakePanel) LoadHTML(ctx context.Context, page string) (string, error) {
	return p.page.HTML(), nil
}

// State is what the view reads.
func (p *fakePanel) State() PanelState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PanelState{Status: p.status, Title: p.title, Loading: p.loading, Loaded: p.loaded}
}

// finishLoad reports that the page settled, which fires the load handler the
// app registered.
func (f *fakePage) finishLoad() {
	f.mu.Lock()
	loaded := f.loaded
	f.mu.Unlock()
	if loaded != nil {
		loaded()
	}
}

// setLoaded points the page's load event at a handler.
func (f *fakePage) setLoaded(fn func()) {
	f.mu.Lock()
	f.loaded = fn
	f.mu.Unlock()
}

// setURL records the address the panel was pointed at.
func (f *fakePage) setURL(page string) {
	f.mu.Lock()
	f.url = page
	f.mu.Unlock()
}

// HTML is the scripted page.
func (f *fakePage) HTML() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.html
}

// URL is the address the panel was pointed at.
func (f *fakePage) URL() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.url == "" {
		return "https://www.69shuba.com/"
	}
	return f.url
}

// setHTML sets the page a test wants the panel to hold.
func (f *fakePage) setHTML(html string) {
	f.mu.Lock()
	f.html = html
	f.mu.Unlock()
}
