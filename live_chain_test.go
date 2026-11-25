package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveFullChain runs 极道天魔 against the live site, end to end.
func TestLiveFullChain(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to run against the live sites")
	}
	useTempDownloads(t)

	source := SourceFor(SourcePiaotian)
	history := NewHistory(filepath.Join(t.TempDir(), "history"))
	page := &fakePage{}
	panel := newFakePanel(page, source)
	session := NewSession(nil, panel, history, source)

	// 1. SEARCH
	*session.Input() = "极道天魔"
	session.Submit()
	waitFor(t, func() bool { return session.Phase() == PhaseIdle }, 60*time.Second)
	hits := session.Hits()
	fmt.Printf("\n[1] 搜索: %d 条\n", len(hits))
	if len(hits) == 0 {
		t.Fatalf("搜索无结果: %q", session.Message())
	}
	for _, h := range hits {
		fmt.Printf("    %s / %s\n        %s\n", h.Title, h.Author, h.URL)
	}

	// 2. CATALOG
	catURL := hits[0].URL
	raw, err := liveGet(catURL)
	if err != nil {
		t.Fatalf("目录页抓取失败: %v", err)
	}
	page.setHTML(decodePage(raw))
	page.setURL(catURL)
	session.pickHit(hits[0])
	page.finishLoad()
	waitFor(t, func() bool { return session.Phase() == PhaseCatalogReady }, 60*time.Second)
	cat := session.Catalog()
	fmt.Printf("\n[2] 目录: 《%s》 %s · %d 章\n", cat.Title, cat.Author, len(cat.Chapters))
	if len(cat.Chapters) == 0 {
		t.Fatalf("目录为空: %q", session.Message())
	}
	fmt.Printf("    首章: %s\n    末章: %s\n", cat.Chapters[0].Title, cat.Chapters[len(cat.Chapters)-1].Title)

	// 3. DOWNLOAD the first 5 chapters off the live site.
	cat.Chapters = cat.Chapters[:5]
	session.startDownload()
	waitFor(t, func() bool {
		p := session.Phase()
		return p == PhaseFinished || p == PhaseFailed
	}, 240*time.Second)
	fmt.Printf("\n[3] 下载: %v · %d 章\n", session.Phase(), len(session.Chapters()))
	total := 0
	for _, c := range session.Chapters() {
		n := len([]rune(c.PlainText))
		total += n
		fmt.Printf("    %03d %-28s %5d 字\n", c.Order, c.Title, n)
	}
	if session.Phase() != PhaseFinished {
		t.Fatalf("下载未完成: %v / %q", session.Phase(), session.Message())
	}
	if len(session.Chapters()) != 5 {
		t.Fatalf("只下载了 %d 章", len(session.Chapters()))
	}
	fmt.Printf("    合计 %d 字\n", total)

	// 4. EXPORT
	saved := session.LastSaved()
	fmt.Printf("\n[4] 导出: %s\n", saved)
	if saved == "" {
		t.Fatal("没有写出 EPUB")
	}
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("EPUB 不是合法 zip: %v", err)
	}
	fmt.Printf("    %d 字节, %d 个条目\n", len(data), len(zr.File))
	for _, f := range zr.File {
		fmt.Printf("      %s\n", f.Name)
	}
	// The real prose has to be inside.
	body := readEntry(t, zr, "OEBPS/chapter-001.xhtml")
	if !strings.Contains(body, "冷风如刀") {
		t.Errorf("第一章正文不在 EPUB 里: %q", firstRunes(body, 200))
	} else {
		fmt.Printf("    第一章正文 ✓ 冷风如刀，大雪纷飞。\n")
	}
	nav := readEntry(t, zr, "OEBPS/nav.xhtml")
	for _, c := range session.Chapters() {
		if !strings.Contains(nav, c.Title) {
			t.Errorf("目录缺少 %q", c.Title)
		}
	}
	fmt.Printf("    目录 5 章 ✓\n")

	// 5. HISTORY
	fmt.Printf("\n[5] 历史: %d 条\n", history.Count())
	for _, s := range history.List() {
		fmt.Printf("    《%s》 %s complete=%v\n", s.Title, s.ProgressLabel(), s.Complete())
	}
}

func liveGet(page string) ([]byte, error) {
	req, err := http.NewRequest("GET", page, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func readEntry(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("%s 不在 EPUB 里", name)
	return ""
}
