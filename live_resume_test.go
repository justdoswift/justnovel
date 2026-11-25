package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLivePauseAndResume pauses a live run, then resumes it from the kept
// chapter, and checks the exported book holds both halves.
func TestLivePauseAndResume(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1")
	}
	useTempDownloads(t)

	source := SourceFor(SourcePiaotian)
	history := NewHistory(filepath.Join(t.TempDir(), "history"))
	page := &fakePage{}
	panel := newFakePanel(page, source)
	session := NewSession(nil, panel, history, source)

	// Catalog from the live site.
	raw, err := liveGet("https://www.piaotia.com/html/8/8502/")
	if err != nil {
		t.Fatal(err)
	}
	page.setHTML(decodePage(raw))
	page.setURL("https://www.piaotia.com/html/8/8502/")
	*session.Input() = "https://www.piaotia.com/html/8/8502/"
	session.Submit()
	page.finishLoad()
	waitFor(t, func() bool { return session.Phase() == PhaseCatalogReady }, 60*time.Second)

	// Keep the catalog short so the run finishes quickly.
	cat := session.Catalog()
	cat.Chapters = cat.Chapters[:8]
	fmt.Printf("\n目录: %d 章\n", len(cat.Chapters))

	// Start, then pause after a moment.
	session.startDownload()
	waitFor(t, func() bool { return len(session.Chapters()) >= 3 }, 60*time.Second)
	session.pauseDownload()

	phase := session.Phase()
	kept := len(session.Chapters())
	fmt.Printf("暂停: phase=%v 保留=%d 章\n", phase, kept)
	if phase != PhasePaused {
		t.Fatalf("暂停后 phase=%v, 应为 PhasePaused", phase)
	}
	if kept == 0 {
		t.Fatal("暂停后没有保留章节")
	}
	if label, _, _ := (&app{session: session}).topAction(); label != "继续下载" {
		t.Errorf("暂停后按钮=%q, 应为 继续下载", label)
	}
	// The pause must be on disk.
	if history.Count() != 1 {
		t.Fatalf("历史记录=%d, 应为 1", history.Count())
	}
	saved := history.List()[0]
	fmt.Printf("历史: %s complete=%v\n", saved.ProgressLabel(), saved.Complete())
	if saved.Downloaded != kept {
		t.Errorf("历史保存 %d 章, 界面显示 %d 章", saved.Downloaded, kept)
	}

	// Resume and let it finish.
	session.resumeDownload()
	waitFor(t, func() bool {
		p := session.Phase()
		return p == PhaseFinished || p == PhaseFailed
	}, 180*time.Second)
	fmt.Printf("续传完成: phase=%v 共 %d 章\n", session.Phase(), len(session.Chapters()))
	if session.Phase() != PhaseFinished {
		t.Fatalf("续传失败: %v / %q", session.Phase(), session.Message())
	}
	if len(session.Chapters()) != 8 {
		t.Fatalf("最终 %d 章, 应为 8", len(session.Chapters()))
	}

	// The EPUB must hold all 8, in order, with real prose.
	data, err := os.ReadFile(session.LastSaved())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	body := readEntry(t, zr, "OEBPS/chapter-001.xhtml")
	if !strings.Contains(body, "冷风如刀") {
		t.Error("第一章正文缺失")
	}
	last := readEntry(t, zr, "OEBPS/chapter-008.xhtml")
	if len([]rune(last)) < 500 {
		t.Errorf("第八章正文过短: %d 字", len([]rune(last)))
	}
	fmt.Printf("EPUB: %d 字节, 8 章 ✓\n", len(data))
}
