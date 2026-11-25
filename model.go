package main

import (
	"net/url"
	"strconv"
	"time"
)

// SourceKind identifies a site JustNovel can read books from.
type SourceKind string

const (
	SourceShuba69  SourceKind = "shuba69"
	SourcePiaotian SourceKind = "piaotian"
	SourceSto9     SourceKind = "sto9"
)

// FontPreset chooses the body font of an exported book.
type FontPreset string

const (
	FontSystem   FontPreset = "system"
	FontPingFang FontPreset = "pingFang"
	FontSongti   FontPreset = "songti"
	FontKaiti    FontPreset = "kaiti"
	FontHeiti    FontPreset = "heiti"
)

// FontPresets lists the body fonts of the export settings, in the order the
// dialog shows them.
var FontPresets = []FontPreset{FontSystem, FontPingFang, FontSongti, FontKaiti, FontHeiti}

// DisplayName is the name the settings show for the preset.
func (f FontPreset) DisplayName() string {
	switch f {
	case FontPingFang:
		return "苹方"
	case FontSongti:
		return "宋体"
	case FontKaiti:
		return "楷体"
	case FontHeiti:
		return "黑体"
	default:
		return "系统字体"
	}
}

// CSSFamily is the font-family the exported stylesheet uses for the preset.
func (f FontPreset) CSSFamily() string {
	switch f {
	case FontPingFang:
		return `"PingFang SC", "Hiragino Sans GB", sans-serif`
	case FontSongti:
		return `"Songti SC", "STSong", serif`
	case FontKaiti:
		return `"Kaiti SC", "STKaiti", serif`
	case FontHeiti:
		return `"Hiragino Sans GB", "Heiti SC", sans-serif`
	default:
		return `-apple-system, BlinkMacSystemFont, "PingFang SC", serif`
	}
}

const defaultFrontPageBodyText = "本 EPUB 由 JustNovel 生成，用于个人离线阅读。"

// ChapterRef is a chapter of a catalog, before its text is downloaded.
type ChapterRef struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Order int    `json:"order"`
}

// Chapter is a downloaded chapter.
type Chapter struct {
	Title     string `json:"title"`
	PlainText string `json:"plainText"`
	Order     int    `json:"order"`
}

// BookCatalog is the table of contents of a book.
type BookCatalog struct {
	Title         string       `json:"title"`
	Author        string       `json:"author"`
	SourceURL     string       `json:"sourceURL"`
	Chapters      []ChapterRef `json:"chapters"`
	BodyFont      FontPreset   `json:"bodyFont"`
	FrontPageText string       `json:"frontPageText"`
	ShowSource    bool         `json:"showSource"`
	CoverImage    []byte       `json:"coverImage,omitempty"`
	CoverType     string       `json:"coverType,omitempty"`
	CoverExt      string       `json:"coverExt,omitempty"`
}

// HasCover reports whether the catalog carries a cover image.
func (c BookCatalog) HasCover() bool { return len(c.CoverImage) > 0 }

// ExportSettings are the title page settings an export uses.
type ExportSettings struct {
	Title      string
	Author     string
	SourceURL  string
	Total      int
	BodyFont   FontPreset
	FrontText  string
	ShowSource bool
	CoverImage []byte
	CoverType  string
	CoverExt   string
}

// Settings builds the export settings of the catalog.
func (c BookCatalog) Settings() ExportSettings {
	return ExportSettings{
		Title:      c.Title,
		Author:     c.Author,
		SourceURL:  c.SourceURL,
		Total:      len(c.Chapters),
		BodyFont:   c.BodyFont,
		FrontText:  c.FrontPageText,
		ShowSource: c.ShowSource,
		CoverImage: c.CoverImage,
		CoverType:  c.CoverType,
		CoverExt:   c.CoverExt,
	}
}

// ApplySettings writes the edited settings back into the catalog.
func (c *BookCatalog) ApplySettings(s ExportSettings) {
	c.Title, c.Author, c.SourceURL = s.Title, s.Author, s.SourceURL
	c.BodyFont, c.FrontPageText, c.ShowSource = s.BodyFont, s.FrontText, s.ShowSource
	c.CoverImage, c.CoverType, c.CoverExt = s.CoverImage, s.CoverType, s.CoverExt
}

// HistoryRecord is one saved download, kept so it can be exported again.
type HistoryRecord struct {
	ID            string         `json:"id"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	LastExported  *time.Time     `json:"lastExportedAt,omitempty"`
	ExportName    string         `json:"lastExportFileName,omitempty"`
	Title         string         `json:"title"`
	Author        string         `json:"author"`
	SourceURL     string         `json:"sourceURL"`
	TotalChapters int            `json:"totalChapters"`
	Settings      ExportSettings `json:"settings"`
	Chapters      []Chapter      `json:"chapters"`
}

// Summary is the record without its chapters: what the history list reads, so
// opening the list never deserializes a whole book.
func (r HistoryRecord) Summary() HistorySummary {
	return HistorySummary{
		ID:            r.ID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
		LastExported:  r.LastExported,
		ExportName:    r.ExportName,
		Title:         r.Title,
		Author:        r.Author,
		SourceURL:     r.SourceURL,
		TotalChapters: r.TotalChapters,
		Downloaded:    len(r.Chapters),
	}
}

// HistorySummary is one row of the history list.
type HistorySummary struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	LastExported  *time.Time `json:"lastExportedAt,omitempty"`
	ExportName    string     `json:"lastExportFileName,omitempty"`
	Title         string     `json:"title"`
	Author        string     `json:"author"`
	SourceURL     string     `json:"sourceURL"`
	TotalChapters int        `json:"totalChapters"`
	Downloaded    int        `json:"downloaded"`
}

// Downloaded counts the chapters the record holds.
func (r HistoryRecord) Downloaded() int { return len(r.Chapters) }

// ProgressLabel is the "done / total" text of the record.
func (r HistoryRecord) ProgressLabel() string { return progressLabel(len(r.Chapters), r.TotalChapters) }

// ProgressLabel is the "done / total" text of the summary.
func (s HistorySummary) ProgressLabel() string { return progressLabel(s.Downloaded, s.TotalChapters) }

func progressLabel(done, total int) string {
	return strconv.Itoa(done) + " / " + strconv.Itoa(total)
}

// SourceHost is the host of the record's page, shown in its details.
func (r HistoryRecord) SourceHost() string { return hostOf(r.SourceURL) }

// SourceHost is the host of the summary's page.
func (s HistorySummary) SourceHost() string { return hostOf(s.SourceURL) }

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

// Complete reports whether every chapter of the book was downloaded.
func (r HistoryRecord) Complete() bool { return len(r.Chapters) >= r.TotalChapters }

// Complete reports whether the summary covers the whole book.
func (s HistorySummary) Complete() bool { return s.Downloaded >= s.TotalChapters }

// Transport is how a chapter page was fetched.
type Transport int

const (
	TransportSession Transport = iota
	TransportBrowser
)

// Title names the transport for the status panel.
func (t Transport) Title() string {
	if t == TransportBrowser {
		return "浏览器回退"
	}
	return "普通请求"
}

// Detail explains the transport.
func (t Transport) Detail() string {
	if t == TransportBrowser {
		return "请求疑似被拦截，已切换到浏览器会话抓取。"
	}
	return "正在直接请求章节页面；遇到短暂网络波动或临时限流时会自动等待并重试。"
}

// Progress is a snapshot of a running download.
type Progress struct {
	Completed    int
	Total        int
	CurrentTitle string
	CurrentIndex int
	Transport    Transport
	LastTitle    string
	LastIndex    int
	HasLast      bool
}

// Fraction is how much of the book is downloaded, from 0 to 1.
func (p Progress) Fraction() float64 {
	if p.Total <= 0 {
		return 0
	}
	done := min(max(p.Completed, 0), p.Total)
	return float64(done) / float64(p.Total)
}

// Phase is the one state the work is in.
//
// Everything the window shows about progress is derived from it. There is no
// separate "searching" or "running" flag: two flags that both mean "busy" is
// exactly how the catalog load used to deadlock against itself.
type Phase int

const (
	PhaseIdle Phase = iota
	PhaseSearching
	PhaseParsing
	PhaseCatalogReady
	PhaseDownloading
	PhasePaused
	PhaseBuilding
	PhaseFinished
	PhaseFailed
	PhaseStopped
)

// Busy reports whether the work blocks a new one from starting. It is the only
// definition of "busy" in the app.
func (p Phase) Busy() bool {
	switch p {
	case PhaseSearching, PhaseParsing, PhaseDownloading, PhaseBuilding:
		return true
	}
	return false
}

// Title is the headline of the status panel.
func (p Phase) Title() string {
	switch p {
	case PhaseSearching:
		return "正在搜索"
	case PhaseParsing:
		return "正在解析目录"
	case PhaseCatalogReady:
		return "目录已就绪"
	case PhaseDownloading:
		return "正在下载章节"
	case PhasePaused:
		return "已暂停"
	case PhaseBuilding:
		return "正在生成 EPUB"
	case PhaseFinished:
		return "流程完成"
	case PhaseStopped:
		return "已停止"
	case PhaseFailed:
		return "需要处理"
	default:
		return "准备开始"
	}
}

// Outcome reports whether the phase is a result the user acts on, rather than
// work in flight.
func (p Phase) Outcome() bool {
	switch p {
	case PhaseCatalogReady, PhasePaused, PhaseFinished, PhaseFailed, PhaseStopped:
		return true
	}
	return false
}
