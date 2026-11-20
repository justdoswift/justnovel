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

// Downloaded counts the chapters the record holds.
func (r HistoryRecord) Downloaded() int { return len(r.Chapters) }

// ProgressLabel is the "done / total" text of the record.
func (r HistoryRecord) ProgressLabel() string {
	return strconv.Itoa(len(r.Chapters)) + " / " + strconv.Itoa(r.TotalChapters)
}

// SourceHost is the host of the record's page, shown in its details.
func (r HistoryRecord) SourceHost() string {
	u, err := url.Parse(r.SourceURL)
	if err != nil || u.Host == "" {
		return r.SourceURL
	}
	return u.Host
}

// Complete reports whether every chapter of the book was downloaded.
func (r HistoryRecord) Complete() bool { return len(r.Chapters) >= r.TotalChapters }

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

// JobPhase is the step a download is in.
type JobPhase int

const (
	PhaseIdle JobPhase = iota
	PhaseCatalogReady
	PhaseSessionRequired
	PhaseParsing
	PhaseStarting
	PhaseDownloading
	PhasePaused
	PhaseBuilding
	PhaseFinished
	PhaseStopped
	PhaseFailed
)

// JobState is the state of the current work, shown in the status panel.
type JobState struct {
	Phase   JobPhase
	Message string
	Detail  string
	Snap    *Progress
}

// Title is the headline of the status panel.
func (j JobState) Title() string {
	switch j.Phase {
	case PhaseCatalogReady:
		return "目录已就绪"
	case PhaseParsing:
		return "正在解析目录"
	case PhaseStarting:
		return "已开始下载"
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
	case PhaseSessionRequired:
		return "等待站点会话"
	default:
		return "准备开始"
	}
}

// Busy reports whether the app is working on something the user waits for.
func (j JobState) Busy() bool {
	switch j.Phase {
	case PhaseParsing, PhaseStarting, PhaseDownloading, PhaseBuilding:
		return true
	}
	return false
}
