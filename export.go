package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// saveDialog asks where to put the EPUB. It is a variable so a test can run
// the export path without a real dialog.
var saveDialog = func(defaultPath string) (string, error) {
	return mygo.Dialog.Save(mygo.SaveDialogOptions{
		Title:       "导出 EPUB",
		DefaultPath: defaultPath,
		Filters:     []mygo.FileFilter{{Name: "EPUB", Extensions: []string{"epub"}}},
	})
}

// beginExport builds the EPUB and writes it. A finished download saves
// straight to Downloads; a manual export asks where to put the file.
func (s *Session) beginExport(settings ExportSettings, chapters []Chapter, recordID string, askPath bool) {
	if len(chapters) == 0 {
		s.update(func() { s.failLocked(errors.New("当前还没有可导出的章节。")) })
		return
	}
	s.update(func() {
		s.setPhaseLocked(PhaseBuilding, "正在生成 EPUB…", "")
		s.logfLocked(LogInfo, "开始生成 EPUB", fmt.Sprintf("%d 章", len(chapters)))
	})
	name := ExportFileName(settings.Title, len(chapters), settings.Total)

	go func() {
		data, err := BuildEPUB(settings, chapters)
		if err != nil {
			s.update(func() { s.failLocked(fmt.Errorf("EPUB 生成失败：%w", err)) })
			return
		}

		path := uniqueDownloadPath(name)
		if askPath {
			chosen, err := saveDialog(path)
			if err != nil {
				s.update(func() { s.failLocked(fmt.Errorf("保存失败：%w", err)) })
				return
			}
			if chosen == "" {
				s.update(func() { s.setPhaseLocked(PhaseFinished, "已取消导出。", "") })
				return
			}
			path = chosen
		}
		if !strings.HasSuffix(strings.ToLower(path), ".epub") {
			path += ".epub"
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			s.update(func() { s.failLocked(fmt.Errorf("保存失败：%w", err)) })
			return
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			s.update(func() { s.failLocked(fmt.Errorf("保存失败：%w", err)) })
			return
		}
		if recordID != "" && s.history != nil {
			s.history.MarkExported(recordID, filepath.Base(path))
		}

		s.update(func() {
			s.lastSaved = path
			s.logfLocked(LogDone, "EPUB 已导出", filepath.Base(path))
			s.setPhaseLocked(PhaseFinished, "EPUB 已导出："+filepath.Base(path), "")
		})
	}()
}
