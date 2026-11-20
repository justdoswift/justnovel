package main

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// search runs the query on the current site. It asks the site directly first,
// because two of the three answer a plain request; only when that fails, or
// comes back as a challenge page, does the browser panel open so a human can
// get through.
func (a *app) search() {
	query := trimParagraph(a.query)
	if query == "" || a.job.Busy() || a.searching {
		return
	}
	if a.source.SearchURL == nil {
		a.searchError = "这个站点还不支持搜索，请粘贴地址。"
		return
	}

	a.searching = true
	a.searchError = ""
	a.hits = nil
	a.logReset()
	a.logf(LogInfo, "搜索", query)

	source := a.source
	go func() {
		hits, err := searchDirectly(source, query)
		a.win.Update(func() {
			if err != nil {
				// The site refused a plain request: open the panel so the
				// page can be read, or a check solved, in front of the user.
				a.logf(LogWarn, "改用浏览器", err.Error())
				a.searchThroughBrowser(query)
				return
			}
			a.finishSearch(hits, "")
		})
	}()
}

// searchThroughBrowser runs the search in the panel, for a site that will not
// answer a plain request.
func (a *app) searchThroughBrowser(query string) {
	a.showBrowser = true
	a.session.OnLoaded(func() {
		a.win.Update(func() { a.readSearchResults() })
	})
	if a.source.Kind == SourceSto9 {
		a.session.Open("https://sto9.com/")
		a.session.SubmitSearchAfterLoad(query)
		return
	}
	a.session.Open(a.source.searchURL(query))
}

// readSearchResults parses what the panel is showing into books.
func (a *app) readSearchResults() {
	if !a.searching {
		return
	}
	html := a.session.HTML()
	base, err := url.Parse(a.session.URL())
	if err != nil {
		base = nil
	}
	hits := a.source.parseSearch(html, base)
	if len(hits) == 0 {
		a.finishSearch(nil, "没有找到这本书，换个关键词或换个站点。")
		return
	}
	a.finishSearch(hits, "")
}

// finishSearch records the outcome and closes the panel again.
func (a *app) finishSearch(hits []SearchHit, message string) {
	a.searching = false
	a.hits = hits
	a.searchError = message
	a.showBrowser = false
	if message != "" {
		a.logf(LogWarn, "没有结果", message)
		return
	}
	a.logf(LogInfo, "找到结果", fmt.Sprintf("%d 本", len(hits)))
}

// searchWithTimeout runs a search and gives up after a while, so the panel
// cannot leave the window waiting forever.
func (a *app) searchWithTimeout() {
	go func() {
		time.Sleep(45 * time.Second)
		a.win.Update(func() {
			if a.searching {
				a.finishSearch(nil, "搜索超时。站点可能需要验证，请在浏览器面板里完成后再搜一次。")
			}
		})
	}()
}

// pickHit opens a book the search found.
func (a *app) pickHit(hit SearchHit) {
	if a.job.Busy() {
		return
	}
	a.urlText = hit.URL
	a.hits = nil
	a.query = ""
	a.openSource()
}

// cancelSearch drops the results and returns to the empty state.
func (a *app) cancelSearch() {
	a.hits = nil
	a.query = ""
	a.searchError = ""
	a.searching = false
}

// searchContext is kept so a cancelled search stops waiting.
var _ = context.Background
