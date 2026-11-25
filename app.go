package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// app is the window: the session it shows, and the view state that belongs to
// the window alone (which dialog is open, what is selected in a list).
//
// It holds no download state. Everything about the work lives in Session, so
// there is one place that knows what the app is doing.
type app struct {
	win     *mygo.Window
	session *Session
	panel   Panel
	history *History

	// showBrowser brings the site's page forward. It is a view preference: the
	// page stays in the tree either way, because a web view that is not placed
	// never loads.
	showBrowser bool

	// searchDraft is the search field's text while a book or the site's page
	// is open: the field stays empty instead of showing the address that
	// opened the book, and a submit copies the draft into the session.
	searchDraft string

	// Draft of the export settings, edited in its dialog.
	showExport   bool
	draft        ExportSettings
	exportFont   string
	coverPreview ui.ImageSource

	showHistory    bool
	historyList    []HistorySummary
	selectedID     string
	historyCover   ui.ImageSource
	historyCoverID string
	historyError   string
	pendingDelete  bool

	showChapters bool
	editChapters bool
	pendingStop  bool

	logState ui.ListState
}

// newApp builds the window state around a session.
func newApp(session *Session, panel Panel, history *History) *app {
	return &app{session: session, panel: panel, history: history}
}

// openExportDialog opens the export settings of the current session.
func (a *app) openExportDialog() {
	if a.session.Catalog() == nil || len(a.session.Chapters()) == 0 {
		return
	}
	a.draft = a.session.Catalog().Settings()
	if trimParagraph(a.draft.FrontText) == "" {
		a.draft.FrontText = defaultFrontPageBodyText
	}
	a.exportFont = a.draft.BodyFont.DisplayName()
	a.showExport = true
	a.refreshCoverPreview()
}

// refreshCoverPreview decodes the draft's cover once, for the dialog.
func (a *app) refreshCoverPreview() {
	a.coverPreview = nil
	if len(a.draft.CoverImage) == 0 {
		return
	}
	if bitmap, err := ui.DecodeBitmap(a.draft.CoverImage); err == nil {
		a.coverPreview = bitmap
	}
}

// confirmExport applies the edited settings and writes the book.
func (a *app) confirmExport() {
	if a.session.Catalog() == nil {
		return
	}
	for _, preset := range FontPresets {
		if preset.DisplayName() == a.exportFont {
			a.draft.BodyFont = preset
		}
	}
	if author := trimParagraph(a.draft.Author); author != "" {
		a.draft.Author = author
	} else {
		a.draft.Author = "Unknown"
	}
	if a.draft.BodyFont == "" {
		a.draft.BodyFont = FontSystem
	}

	catalog := *a.session.Catalog()
	catalog.ApplySettings(a.draft)
	a.session.catalog = &catalog
	a.showExport = false

	a.session.beginExport(catalog.Settings(), slices.Clone(a.session.Chapters()), a.session.recordID, true)
}

// importCover asks for an image and makes it the book's cover.
func (a *app) importCover() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:   "选择封面",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "webp"}, MIMETypes: []string{"image/*"}}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	if err != nil || len(data) == 0 {
		return
	}
	mediaType, ext := coverTypeFor(paths[0])
	a.draft.CoverImage = data
	a.draft.CoverType = mediaType
	a.draft.CoverExt = ext
	a.refreshCoverPreview()
}

// removeCover clears the draft's cover.
func (a *app) removeCover() {
	a.draft.CoverImage = nil
	a.draft.CoverType = ""
	a.draft.CoverExt = ""
	a.coverPreview = nil
}

// openHistory loads the saved downloads into the dialog.
func (a *app) openHistory() {
	a.historyList = a.history.List()
	a.showHistory = true
	a.historyError = ""
	if a.selectedID == "" || !a.hasRecord(a.selectedID) {
		a.selectedID = ""
		if len(a.historyList) > 0 {
			a.selectedID = a.historyList[0].ID
		}
	}
	a.selectHistory(a.selectedID)
}

// openHistoryRecord opens the history dialog on one record, for a row of the
// sidebar's recent list.
func (a *app) openHistoryRecord(id string) {
	a.selectedID = id
	a.openHistory()
}

// selectHistory chooses a record and decodes its cover once.
func (a *app) selectHistory(id string) {
	a.selectedID = id
	if id == a.historyCoverID {
		return
	}
	a.historyCoverID = id
	a.historyCover = nil
	record, ok := a.selectedRecord()
	if !ok || len(record.Settings.CoverImage) == 0 {
		return
	}
	if bitmap, err := ui.DecodeBitmap(record.Settings.CoverImage); err == nil {
		a.historyCover = bitmap
	}
}

// hasRecord reports whether the list holds a record.
func (a *app) hasRecord(id string) bool {
	for _, record := range a.historyList {
		if record.ID == id {
			return true
		}
	}
	return false
}

// selectedRecord is the record the dialog shows, with its chapters.
func (a *app) selectedRecord() (HistoryRecord, bool) {
	if a.selectedID == "" {
		return HistoryRecord{}, false
	}
	return a.history.Get(a.selectedID)
}

// exportHistory writes a saved record again.
func (a *app) exportHistory() {
	record, ok := a.selectedRecord()
	if !ok || len(record.Chapters) == 0 {
		return
	}
	settings := record.Settings
	if settings.Total == 0 {
		settings.Total = record.TotalChapters
	}
	a.showHistory = false
	a.session.beginExport(settings, record.Chapters, record.ID, true)
}

// deleteHistory removes the selected record.
func (a *app) deleteHistory() {
	if a.selectedID == "" {
		return
	}
	if err := a.history.Delete(a.selectedID); err != nil {
		a.historyError = "删除历史记录失败：" + err.Error()
		return
	}
	a.historyList = a.history.List()
	a.selectedID = ""
	if len(a.historyList) > 0 {
		a.selectedID = a.historyList[0].ID
	}
	a.selectHistory(a.selectedID)
	a.pendingDelete = false
}

// recentRecords is the short list the empty screen shows.
func (a *app) recentRecords(limit int) []HistorySummary {
	if a.history == nil {
		return nil
	}
	records := a.history.List()
	if len(records) > limit {
		records = records[:limit]
	}
	return records
}

// humanBytes formats a byte count the way Finder does.
func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// downloadsDir is where a finished book is saved. It is a variable so a test
// can point an export at a temporary directory instead of writing into the
// user's real Downloads folder.
var downloadsDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// uniqueDownloadPath picks a Downloads path that does not overwrite an
// earlier export.
func uniqueDownloadPath(name string) string {
	dir := downloadsDir()
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	path := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); err != nil {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
	}
}
