package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// searchDirectly asks a site for its search results with a plain request. Two
// of the three sites answer it; 69shuba answers with a Cloudflare challenge,
// which is reported as an error so the caller can fall back to the browser.
func searchDirectly(source Source, query string) ([]SearchHit, error) {
	target := source.searchURL(query)
	if target == "" {
		return nil, errors.New("这个站点还不支持搜索。")
	}

	var req *http.Request
	var err error
	if source.Kind == SourceSto9 {
		// 思兔 takes a POST form.
		req, err = http.NewRequest("POST", target, nil)
		if err != nil {
			return nil, err
		}
		req.URL.RawQuery = ""
		req.PostForm = sto9SearchForm(query)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		body := sto9SearchForm(query).Encode()
		req, err = http.NewRequest("POST", target, io.NopCloser(stringReader(body)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest("GET", target, nil)
		if err != nil {
			return nil, err
		}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	page := decodePage(raw)

	if resp.StatusCode == http.StatusForbidden || looksLikeChallengePage(page) {
		return nil, fmt.Errorf("%s 要求验证，改用浏览器", source.Name)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s 返回了 %d", source.Name, resp.StatusCode)
	}

	base, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	hits := source.parseSearch(page, base)
	if len(hits) == 0 {
		return nil, fmt.Errorf("%s 没有返回结果", source.Name)
	}
	return hits, nil
}

// stringReader avoids importing strings just for one reader.
func stringReader(s string) io.Reader {
	return &readerOf{s: s}
}

type readerOf struct {
	s string
	i int
}

func (r *readerOf) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
