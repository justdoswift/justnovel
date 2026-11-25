package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// searchDirectly asks a site for its search results with a plain request.
//
// It reports three things: the books it found, whether the site refused the
// request so the browser has to take over, and a real error. An empty result
// is not an error and does not ask for the browser: the site answered, it just
// has no such book.
func searchDirectly(ctx context.Context, source Source, query string) ([]SearchHit, bool, error) {
	target := source.searchURL(query)
	if target == "" {
		return nil, false, errors.New("这个站点还不支持搜索。")
	}

	req, err := buildSearchRequest(ctx, source, target, query)
	if err != nil {
		return nil, false, err
	}

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, false, err
	}
	page := decodePage(raw)

	if resp.StatusCode == http.StatusForbidden || looksLikeChallengePage(page) {
		return nil, true, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("%s 返回了 %d", source.Name, resp.StatusCode)
	}

	base, err := url.Parse(target)
	if err != nil {
		return nil, false, err
	}
	return source.parseSearch(page, base), false, nil
}

// buildSearchRequest builds the request the site's own form would send. 思兔
// takes a POST form; the other two take the query in the URL.
func buildSearchRequest(ctx context.Context, source Source, target, query string) (*http.Request, error) {
	var req *http.Request
	var err error
	if source.Kind == SourceSto9 {
		body := sto9SearchForm(query).Encode()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	return req, nil
}
