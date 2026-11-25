package main

import (
	"context"
	"net/http"

	"github.com/egoist/mygo"
)

// Panel is the site's browser, as the session uses it: the page a human
// verifies in, the cookies it holds, and the pages it can fetch when the plain
// HTTP client may not.
//
// It is an interface so the catalog and download flows can be tested against a
// scripted page. The real one is browserPanel.
type Panel interface {
	// Configure points the panel at a site.
	Configure(source Source)
	// Open shows a page.
	Open(page string)
	// OnLoaded sets what runs when a page settles.
	OnLoaded(fn func())
	// ClearLoaded drops the load handler.
	ClearLoaded()
	// OnManual sets what runs when the site demands a human.
	OnManual(fn func())
	// SubmitSearchAfterLoad fills the site's own search form, for a POST
	// search.
	SubmitSearchAfterLoad(query string)
	// HTML is the page the panel shows.
	HTML() string
	// URL is the address the panel shows.
	URL() string
	// Sync reads the panel's cookies and page.
	Sync(ctx context.Context) (html string, cookies []*http.Cookie, agent string, err error)
	// Cookies are the site's cookies, HTTPOnly ones included.
	Cookies() []*http.Cookie
	// Agent is the browser's user agent.
	Agent() string
	// LoadHTML fetches a page through the browser, for the crawler's fallback.
	LoadHTML(ctx context.Context, page string) (string, error)
	// State is what the view reads of the panel.
	State() PanelState
	// WebView is the native view the window places, nil when there is none.
	WebView() *mygo.WebView
}
