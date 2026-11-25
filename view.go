package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The visual system: a native macOS window, as System Settings draws one. A
// full-height translucent sidebar holds the library and the sites; the content
// pane carries a unified toolbar with the book's identity, the search field
// and the one primary action; the work itself is set in system type, with
// hairlines instead of boxes. Blue is the only accent, red belongs to the seal
// and to destructive actions.
var (
	contentBg  = ui.Hex("#ffffff")
	sidebarBg  = ui.Hex("#f5f5f7")
	fill       = ui.Hex("#f2f2f7")
	fillStrong = ui.Hex("#e8e8ed")

	textMain  = ui.Hex("#1d1d1f")
	textSoft  = ui.Hex("#6e6e73")
	textFaint = ui.Hex("#86868b")

	// The sidebar's material is a little darker than the content, so its
	// small text takes a darker secondary than textSoft.
	textSidebar = ui.Hex("#56565b")

	separator       = ui.RGBA(0, 0, 0, 0.08)
	separatorStrong = ui.RGBA(0, 0, 0, 0.16)

	accent    = ui.Hex("#007aff")
	accentInk = ui.Hex("#ffffff")
	seal      = ui.Hex("#b3402f")
	danger    = ui.Hex("#ff3b30")
	warning   = ui.Hex("#ff9500")
	success   = ui.Hex("#34c759")

	// Text-sized state colors: the bright system colors are for dots, icons
	// and bars, which need only 3:1, not for 11–13pt text.
	dangerText  = ui.Hex("#d70015")
	warningText = ui.Hex("#8a4b00")
	successText = ui.Hex("#1e7a34")

	selection   = ui.RGBA(0, 122, 255, 0.12)
	progressBg  = ui.RGBA(0, 0, 0, 0.08)
	sheetShadow = ui.RGBA(0, 0, 0, 0.18)

	// A source-list row chosen in an inactive window: a neutral gray darker
	// than the material, with a semibold label, rather than the accent.
	inactiveSelection = ui.Hex("#cfcecc")
)

var theme = func() *ui.Theme {
	t := ui.LightTheme()
	t.Background = contentBg
	t.Surface = contentBg
	t.SurfaceHover = fill
	t.SurfacePressed = fillStrong
	t.Border = separatorStrong
	t.Text = textMain
	t.TextMuted = textFaint
	t.Accent = accent
	t.AccentHover = ui.Hex("#0068d9")
	t.AccentPressed = ui.Hex("#0057b8")
	t.AccentText = accentInk
	t.InactiveAccent = ui.Hex("#d1d1d6")
	t.Danger = danger
	t.Warning = warning
	t.Success = success
	t.Selection = selection
	t.Focus = ui.RGBA(0, 122, 255, 0.35)
	t.FocusRingWidth = 3
	t.FocusRingOffset = 2
	t.Inverse = textMain
	t.InverseText = contentBg
	t.Radius = 6
	t.Spacing = 4
	t.FontSize = 13
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.20)
	return t
}()

// One stroke weight, round caps, drawn on a 16 grid — the whole icon set.
var (
	iconCheck   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3.2 8.6 6.6 12 12.8 4.6"/></svg>`))
	iconRetry   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 8a5 5 0 1 1-1.5-3.6"/><path d="M13 2.8v3.4h-3.4"/></svg>`))
	iconSwap    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 5.5h9M9 2.8l2.7 2.7L9 8.2"/><path d="M13.5 10.5h-9M7 7.8l-2.7 2.7L7 13.2"/></svg>`))
	iconWarn    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2.4 14.6 13H1.4L8 2.4z"/><path d="M8 6.6v3.2"/><path d="M8 11.4h.01"/></svg>`))
	iconError   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="8" r="5.6"/><path d="M8 5.4v3.4"/><path d="M8 11h.01"/></svg>`))
	iconInfo    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="8" r="5.6"/><path d="M8 7.4v3.2"/><path d="M8 5h.01"/></svg>`))
	iconChevron = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3.5 10.5 8 6 12.5"/></svg>`))
	iconHistory = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="8" r="5.6"/><path d="M8 4.8V8l2.2 1.4"/></svg>`))
	iconGlobe   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="8" r="5.6"/><path d="M2.6 8h10.8"/><path d="M8 2.4c1.7 1.6 2.6 3.5 2.6 5.6S9.7 12 8 13.6C6.3 12 5.4 10.1 5.4 8S6.3 4 8 2.4z"/></svg>`))
	iconMore    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><circle cx="3.2" cy="8" r="1.2"/><circle cx="8" cy="8" r="1.2"/><circle cx="12.8" cy="8" r="1.2"/></svg>`))
	iconBookBig = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 7h26a4.5 4.5 0 0 1 4.5 4.5V42H13.5A4.5 4.5 0 0 1 9 37.5z"/><path d="M9 37.5A4.5 4.5 0 0 1 13.5 33H39.5"/><path d="M16.5 7v26"/><path d="M22.5 15h10"/></svg>`))
)

// view draws the window: the library on the left, the work on the right.
func (a *app) view(c *ui.Context) {
	c.SetTheme(theme)

	// The material shows where the view draws no background: the sidebar.
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}

	bar := c.TitleBar()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, bar)
		ui.Box(c).Width(1).Shrink(0).Background(separator)
		a.pane(c)
	})

	a.exportDialog(c)
	a.historyDialog(c)
	a.chaptersDialog(c)
	a.stopAlert(c)
	a.deleteAlert(c)
}

// sidebar is the library: the recent downloads and the sites, as the source
// list of a native window, with the window's material showing through.
func (a *app) sidebar(c *ui.Context, bar ui.TitleBar) {
	top := bar.Bottom + 10
	if top < 16 {
		top = 16
	}

	column := ui.Column(c).Width(228).Shrink(0).Padding(top, 10, 12, 10).Gap(0)
	if !c.Vibrancy() {
		column.Background(sidebarBg)
	}
	column.Children(func() {
		a.brand(c)
		ui.Box(c).Height(18).Shrink(0)

		a.sidebarRecent(c)
		ui.Box(c).Height(16).Shrink(0)
		a.sidebarSources(c)

		ui.Spacer(c)
		ui.Text(c, "仅抓取你有权访问的公开页面").
			FontSize(11).TextColor(textSidebar).Padding(0, 8).MaxLines(2).LineHeight(1.4)
	})
}

func (a *app) brand(c *ui.Context) {
	ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Column(c).Size(20, 20).Radius(5).Background(seal).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "J").FontSize(11).FontWeight(600).TextColor(accentInk)
		})
		ui.Text(c, "justnovel").FontSize(13).FontWeight(600).TextColor(textMain)
	})
}

// sidebarSection is the small gray heading of a group of sidebar rows.
func (a *app) sidebarSection(c *ui.Context, title string, count int) {
	ui.Row(c).Height(24).Padding(0, 8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(textSidebar).Grow(1)
		if count > 0 {
			ui.Text(c, itoa(count)).FontSize(11).TextColor(textSidebar).FontFeatures("tnum")
		}
	})
}

// sidebarRecent is the last few books, as a way back into earlier work.
func (a *app) sidebarRecent(c *ui.Context) {
	records := a.recentRecords(5)
	count := 0
	if a.history != nil {
		count = a.history.Count()
	}

	ui.Column(c).Gap(1).Children(func() {
		a.sidebarSection(c, "最近下载", count)

		if len(records) == 0 {
			ui.Text(c, "暂无记录").FontSize(12).TextColor(textSidebar).Padding(4, 8)
			return
		}

		for _, record := range records {
			record := record
			row := ui.Row(c).Height(28).Padding(4, 8).Radius(6).Gap(7).AlignItems(ui.Center).
				OnClick(func() { a.openHistoryRecord(record.ID) })
			if row.Hovered() {
				row.Background(fill)
			}
			row.Children(func() {
				typeCover(c, record.Title, 16, 20, 9, "")
				ui.Text(c, record.Title).FontSize(13).TextColor(textMain).Grow(1).MaxLines(1)
			})
		}

		row := ui.Row(c).Height(28).Padding(4, 8).Radius(6).Gap(7).AlignItems(ui.Center).
			OnClick(a.openHistory)
		if row.Hovered() {
			row.Background(fill)
		}
		row.Children(func() {
			ui.Icon(c, iconHistory).Size(14, 14).TextColor(textSoft)
			ui.Text(c, "全部历史").FontSize(13).TextColor(textMain)
		})
	})
}

// sidebarSources is the site the next book comes from, as the chosen row of a
// source list: filled with the accent while the window is active, gray while
// it is not.
func (a *app) sidebarSources(c *ui.Context) {
	current := a.session.Source().Kind

	ui.Column(c).Gap(1).Children(func() {
		a.sidebarSection(c, "站点", 0)

		for _, source := range Sources {
			source := source
			selected := source.Kind == current
			row := ui.Row(c).Height(28).Padding(4, 8).Radius(6).Gap(7).AlignItems(ui.Center).
				OnClick(func() { a.session.adopt(source) })

			iconColor, labelColor := textSoft, textMain
			switch {
			case selected && c.WindowActive():
				row.Background(accent)
				iconColor, labelColor = accentInk, accentInk
			case selected:
				row.Background(inactiveSelection)
				iconColor, labelColor = textMain, textMain
			case row.Hovered():
				row.Background(fill)
			}

			row.Children(func() {
				ui.Icon(c, iconGlobe).Size(14, 14).TextColor(iconColor)
				label := ui.Text(c, source.Name).FontSize(13).TextColor(labelColor).Grow(1).MaxLines(1)
				if selected && !c.WindowActive() {
					label.FontWeight(600)
				}
			})
		}
	})
}

// pane is the work: the unified toolbar across the top and the stage under it.
func (a *app) pane(c *ui.Context) {
	ui.Column(c).Grow(1).MinWidth(0).Background(contentBg).Children(func() {
		a.toolbar(c)
		a.stage(c)
	})
}

// toolbar holds the identity of what is open, the one input, and the action
// that moves the work forward, as a native toolbar: identity at the leading
// edge, controls at the trailing one.
func (a *app) toolbar(c *ui.Context) {
	ui.Column(c).Shrink(0).Children(func() {
		ui.Row(c).Height(52).Padding(0, 20, 0, 20).Gap(8).AlignItems(ui.Center).
			DragWindow().Children(func() {
			a.toolbarIdentity(c)
			ui.Spacer(c)

			a.searchField(c)

			a.toolbarIconButton(c, iconHistory, "历史", textSoft, a.openHistory)

			if a.panel != nil && a.panel.State().Loaded {
				label := "浏览器"
				if a.panelVisible() {
					label = "收起浏览器"
				}
				a.toolbarIconButton(c, iconGlobe, label, textSoft, func() { a.showBrowser = !a.panelVisible() })
			}
			a.topActionButton(c)
		})
		ui.Divider(c).Background(separator)
	})
}

// searchField is the one input. While a book or the site's page is in front it
// holds a fresh query rather than the address that opened the book, the way a
// document window keeps its search field empty until it is used.
func (a *app) searchField(c *ui.Context) {
	query := a.session.Input()
	if a.session.Catalog() != nil || a.panelVisible() {
		query = &a.searchDraft
	}

	field := ui.SearchField(c, query).
		Placeholder("搜索书名或地址").
		FontSize(13).Height(28).Width(240).Padding(0, 8).
		Background(fill).Border(0, ui.Transparent).Radius(6)
	if field.Submitted() {
		if query != a.session.Input() {
			*a.session.Input() = *query
			a.searchDraft = ""
		}
		a.session.Submit()
	}
}

// toolbarIdentity is the book or screen in the toolbar: a small cover when a
// book is open, its name, and one line of context under it.
func (a *app) toolbarIdentity(c *ui.Context) {
	title, subtitle := a.screenTitle()

	ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
		if a.session.Catalog() != nil {
			catalog := a.session.Catalog()
			typeCover(c, catalog.Title, 26, 34, 14, "")
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(textMain).MaxLines(1)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(11).TextColor(textSoft).MaxLines(1)
			}
		})
	})
}

// screenTitle names what the window is showing: the book, the search, the
// site's page, or the empty library.
func (a *app) screenTitle() (title, subtitle string) {
	s := a.session
	switch {
	case a.panelVisible():
		return "浏览器", s.Source().Name
	case len(s.Hits()) > 0:
		query := trimParagraph(*s.Input())
		return "搜索结果", fmt.Sprintf("「%s」", query)
	case s.Catalog() != nil:
		catalog := s.Catalog()
		parts := make([]string, 0, 2)
		if catalog.Author != "" && catalog.Author != "Unknown" {
			parts = append(parts, catalog.Author)
		}
		parts = append(parts, s.Source().Name)
		return catalog.Title, strings.Join(parts, " · ")
	default:
		return "书库", "把网络小说下载成 EPUB"
	}
}

// toolbarButton is a borderless button for the toolbar and the status bar: it
// takes a face only while the pointer is on it.
func (a *app) toolbarButton(c *ui.Context, label string, color ui.Color, action func()) {
	b := ui.Button(c, label).FontSize(12).Height(28).Padding(0, 10).Radius(6).
		Background(ui.Transparent).Border(0, ui.Transparent).TextColor(color)
	if b.Hovered() {
		b.Background(fill)
	}
	if b.Clicked() {
		action()
	}
}

// toolbarIconButton is a toolbar control of a drawn icon and a short label,
// as the history and browser controls in the window's toolbar. It is a real
// button, so it takes the keyboard focus and assistive technology finds it.
func (a *app) toolbarIconButton(c *ui.Context, icon *ui.SVG, label string, color ui.Color, action func()) {
	b := ui.Button(c, "").Label(label).FontSize(12).Height(28).Padding(0, 8).Radius(6).
		Background(ui.Transparent).Border(0, ui.Transparent)
	if b.Hovered() {
		b.Background(fill)
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(13, 13).TextColor(color)
			ui.Text(c, label).FontSize(12).TextColor(color)
		})
	})
	if b.Clicked() {
		action()
	}
}

// miniButton is the smaller borderless button of a list header.
func (a *app) miniButton(c *ui.Context, label string, color ui.Color, action func()) {
	b := ui.Button(c, label).FontSize(11).Height(24).Padding(0, 8).Radius(6).
		Background(ui.Transparent).Border(0, ui.Transparent).TextColor(color)
	if b.Hovered() {
		b.Background(fill)
	}
	if b.Clicked() {
		action()
	}
}

// topAction is the one primary action, named for the next step. The label
// follows the phase, so the button never offers to export a book that was
// already exported.
func (a *app) topAction() (string, bool, func()) {
	s := a.session
	switch {
	case s.CanPause():
		return "暂停下载", true, s.pauseDownload
	case s.CanResume():
		return "继续下载", true, s.resumeDownload
	case s.CanStart():
		return "开始下载", true, s.startDownload
	case s.phase == PhaseFinished && len(s.chapters) > 0:
		return "再导出一份", true, a.openExportDialog
	case s.Busy():
		return "处理中…", false, nil
	default:
		// Nothing to do from the bar: the middle of the window holds the
		// input, and the bar stays quiet until a run starts.
		return "", false, nil
	}
}

func (a *app) topActionButton(c *ui.Context) {
	label, enabled, action := a.topAction()
	if label == "" {
		return
	}
	if enabled {
		ui.PrimaryButton(c, label).FontSize(13).Height(28).Padding(0, 12).
			OnClick(action)
		return
	}
	ui.Button(c, label).FontSize(13).Height(28).Padding(0, 12).
		Background(fill).Border(0, ui.Transparent).TextColor(textFaint).Disabled(true)
}

// stage is the work in the middle: the results, the book, or the run. The
// browser panel is placed once, by panelSlot, whichever stage is up.
func (a *app) stage(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		switch {
		case a.panelVisible():
			a.browserStage(c)
		case len(a.session.Hits()) > 0:
			a.resultsStage(c)
		case a.session.Catalog() != nil:
			a.bookStage(c)
		default:
			a.emptyStage(c)
		}
	})
}

// panelVisible reports whether the site's page is in front. The session says
// when a human is needed; the window's own toggle can only put it away, never
// bring it back on its own.
func (a *app) panelVisible() bool {
	if a.session.WantsPanel() {
		return true
	}
	return a.showBrowser && a.panel != nil && a.panel.State().Loaded
}

// browserStage is the site's own page, in front, while a human is needed.
func (a *app) browserStage(c *ui.Context) {
	var state PanelState
	if a.panel != nil {
		state = a.panel.State()
	}
	title := state.Title
	if title == "" {
		title = a.session.Source().Name
	}

	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(40).Padding(0, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconGlobe).Size(13, 13).TextColor(textFaint)
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(textMain).MaxLines(1)
			ui.Text(c, state.Status).FontSize(11).TextColor(textSoft).MaxLines(1)
			ui.Spacer(c)
		})
		ui.Divider(c).Background(separator)
		a.panelSlot(c)
	})
}

// panelSlot places the site's web view. It is the only place that does: the
// old view placed the same web view twice when a search was running with the
// panel hidden, and two placements of one web view is undefined.
func (a *app) panelSlot(c *ui.Context) {
	if a.panel == nil || a.panel.WebView() == nil {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "浏览器会话不可用。").FontSize(13).TextColor(textSoft)
		})
		return
	}
	if a.panelVisible() {
		ui.WebView(c, a.panel.WebView()).Grow(1).MinHeight(0)
		return
	}
	// Hidden but placed: a web view that is not in the tree never loads
	// anything, and both searching and reading a catalog go through it.
	ui.Box(c).Absolute().Top(0).Left(0).Size(2, 2).Opacity(0.01).Children(func() {
		ui.WebView(c, a.panel.WebView()).Size(2, 2)
	})
}

// emptyStage is the first screen: one quiet invitation, since the search
// field is always in the toolbar.
func (a *app) emptyStage(c *ui.Context) {
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(420).AlignItems(ui.Center).Gap(0).Children(func() {
			ui.Icon(c, iconBookBig).Size(48, 48).TextColor(textFaint)
			ui.Box(c).Height(16).Shrink(0)
			ui.Text(c, "搜一本书").FontSize(17).FontWeight(600).TextColor(textMain)
			ui.Box(c).Height(6).Shrink(0)
			ui.Text(c, "输入书名，回车。也可以粘贴书籍页或目录页地址。").
				FontSize(13).TextColor(textSoft).TextAlign(ui.Center).LineHeight(1.5)

			if message := a.session.Message(); message != "" {
				ui.Box(c).Height(10).Shrink(0)
				ui.Text(c, message).FontSize(12).TextColor(a.messageColor()).
					TextAlign(ui.Center).MaxLines(3).LineHeight(1.5)
			}
		})
	})
}

// messageColor is the tone of the status message: an error reads as one.
func (a *app) messageColor() ui.Color {
	if a.session.phase == PhaseFailed {
		return dangerText
	}
	return textSoft
}

// listHeader is the small heading above a list, with an optional count and a
// quiet action at the trailing edge.
func (a *app) listHeader(c *ui.Context, title, trailing, actionLabel string, action func()) {
	ui.Row(c).Height(36).Padding(0, 20).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(textSoft)
		if trailing != "" {
			ui.Text(c, trailing).FontSize(11).TextColor(textSoft).FontFeatures("tnum")
		}
		ui.Spacer(c)
		if actionLabel != "" && action != nil {
			a.miniButton(c, actionLabel, textSoft, action)
		}
	})
}

// resultsStage is what the search found: pick one, and it opens.
func (a *app) resultsStage(c *ui.Context) {
	hits := a.session.Hits()

	ui.Column(c).Fill().Children(func() {
		a.listHeader(c, fmt.Sprintf("%d 个结果", len(hits)), "", "清除", a.session.cancelSearch)
		ui.Divider(c).Background(separator)

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				for i, hit := range hits {
					hit := hit
					row := ui.Row(c).Height(44).Padding(0, 20).Gap(12).
						AlignItems(ui.Center).OnClick(func() { a.session.pickHit(hit) })
					if row.Hovered() {
						row.Background(fill)
					}
					row.Children(func() {
						ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
							ui.Text(c, hit.Title).FontSize(13).FontWeight(500).
								TextColor(textMain).MaxLines(1)
							if hit.Author != "" {
								ui.Text(c, hit.Author).FontSize(11).TextColor(textSoft).MaxLines(1)
							}
						})
						ui.Icon(c, iconChevron).Size(11, 11).TextColor(textFaint)
					})
					if i < len(hits)-1 {
						ui.Divider(c).Background(separator).Margin(0, 20, 0, 20)
					}
				}
			})
		})
	})
}

// bookStage is the book once its catalog is read: its progress, its contents,
// and the run once it starts.
func (a *app) bookStage(c *ui.Context) {
	catalog := a.session.Catalog()

	ui.Column(c).Fill().Children(func() {
		if a.showProgress() {
			a.progressBand(c, a.session.Progress())
		}

		switch {
		case a.session.Phase() == PhaseCatalogReady || len(a.session.Log()) == 0:
			a.catalogPreview(c, catalog)
		default:
			log := a.session.Log()
			a.listHeader(c, "下载记录", fmt.Sprintf("%d 章", len(a.session.Chapters())), "", nil)
			ui.Divider(c).Background(separator)
			ui.List(c, &a.logState, len(log), func(i int) {
				a.logRow(c, log[i])
			}).Grow(1).MinHeight(0)
		}

		if a.showRail() {
			ui.Divider(c).Background(separator)
			a.rail(c)
		}
	})
}

// showProgress reports whether the run has progress worth a band.
func (a *app) showProgress() bool {
	if a.session.Progress() == nil {
		return false
	}
	switch a.session.Phase() {
	case PhaseDownloading, PhasePaused:
		return true
	case PhaseFailed:
		return len(a.session.Chapters()) > 0
	}
	return false
}

// progressBand is how far the run has got: the count, the percentage, the pace
// and the remaining time over one thin native progress bar.
func (a *app) progressBand(c *ui.Context, snap *Progress) {
	pct := int(snap.Fraction()*100 + 0.5)

	ui.Column(c).Padding(10, 20).Gap(6).Children(func() {
		ui.Row(c).AlignItems(ui.Baseline).Gap(6).Children(func() {
			ui.Text(c, itoa(snap.Completed)).FontSize(15).FontWeight(600).
				TextColor(textMain).FontFeatures("tnum")
			ui.Textf(c, "/ %d 章", snap.Total).FontSize(11).
				TextColor(textSoft).FontFeatures("tnum")
			ui.Textf(c, "%d%%", pct).FontSize(11).
				TextColor(textSoft).FontFeatures("tnum")
			ui.Spacer(c)
			if a.session.Phase() == PhaseDownloading {
				if pace := a.paceLabel(snap); pace != "" {
					ui.Text(c, pace).FontSize(11).TextColor(textSoft).FontFeatures("tnum")
				}
				if eta := a.etaLabel(snap); eta != "" {
					ui.Text(c, "剩余 "+eta).FontSize(11).TextColor(textSoft).FontFeatures("tnum")
				}
			}
		})
		ui.Progress(c, snap.Fraction()).Height(3).Radius(1.5).
			Background(progressBg).TextColor(accent)
	})
	ui.Divider(c).Background(separator)
}

// catalogPreview is the book's contents before a run.
func (a *app) catalogPreview(c *ui.Context, catalog *BookCatalog) {
	chapters := catalog.Chapters

	a.listHeader(c, "目录", fmt.Sprintf("共 %d 章", len(chapters)), "", nil)
	ui.Divider(c).Background(separator)

	ui.List(c, nil, len(chapters), func(i int) {
		chapter := chapters[i]
		ui.Row(c).Height(32).Padding(0, 20).AlignItems(ui.Center).Children(func() {
			ui.Text(c, chapter.Title).FontSize(13).TextColor(textMain).Grow(1).MaxLines(1)
		})
		ui.Divider(c).Background(separator).Margin(0, 20, 0, 20)
	}).Grow(1).MinHeight(0)
}

// logIcon picks the mark for one line of the run: a drawn icon per kind,
// never a text glyph, so the column reads at a glance.
func logIcon(kind LogKind) (icon *ui.SVG, color ui.Color) {
	switch kind {
	case LogChapter:
		return iconCheck, textFaint
	case LogDone:
		return iconCheck, successText
	case LogRetry:
		return iconRetry, warningText
	case LogFallback:
		return iconSwap, warningText
	case LogWarn:
		return iconWarn, warningText
	case LogError:
		return iconError, dangerText
	}
	return iconInfo, textFaint
}

// logRow is one line of the run: when, what, and how long it took.
func (a *app) logRow(c *ui.Context, line LogLine) {
	icon, markColor := logIcon(line.Kind)
	textColor := textSoft
	switch line.Kind {
	case LogChapter, LogDone, LogFallback:
		textColor = textMain
	case LogError:
		textColor = dangerText
	case LogWarn, LogRetry:
		textColor = warningText
	}
	detailColor := textSoft
	switch line.Kind {
	case LogError:
		detailColor = dangerText
	case LogWarn, LogRetry, LogFallback:
		detailColor = warningText
	}

	text := line.Text
	if line.Kind == LogChapter {
		// The line leads with the chapter's order, and the title usually
		// carries it again; keep the title alone.
		if i := strings.Index(text, "  "); i > 0 {
			digits := true
			for _, r := range text[:i] {
				if r < '0' || r > '9' {
					digits = false
					break
				}
			}
			if digits {
				text = text[i+2:]
			}
		}
	}

	ui.Row(c).Height(28).Padding(0, 20).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, line.At.Format("15:04:05")).FontSize(11).
			TextColor(textSoft).Width(52).FontFeatures("tnum")
		ui.Icon(c, icon).Size(12, 12).TextColor(markColor).Width(16)
		ui.Text(c, text).FontSize(12).TextColor(textColor).Grow(1).MaxLines(1)
		if line.Detail != "" {
			ui.Text(c, line.Detail).FontSize(11).TextColor(detailColor).MaxLines(1)
		}
		if line.Took > 0 {
			ui.Text(c, shortDuration(line.Took)).FontSize(11).
				TextColor(textSoft).Width(44).TextAlign(ui.End).FontFeatures("tnum")
		}
	})
}

// rail is the status bar at the bottom: what the run is doing now, or how it
// ended, and what can be done about it.
func (a *app) rail(c *ui.Context) {
	s := a.session

	ui.Row(c).Height(52).Padding(0, 20).Gap(10).AlignItems(ui.Center).Children(func() {
		switch {
		case s.phase == PhaseDownloading:
			snap := s.Progress()
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, "正在下载").FontSize(12).FontWeight(600).TextColor(textMain)
			if snap != nil {
				ui.Text(c, snap.CurrentTitle).FontSize(12).TextColor(textSoft).
					Grow(1).MaxLines(1)
				if snap.Transport == TransportBrowser {
					ui.Text(c, "浏览器").FontSize(11).TextColor(warningText)
				}
			} else {
				ui.Spacer(c)
			}
		case s.phase.Outcome():
			ui.Box(c).Size(7, 7).Radius(3.5).Shrink(0).Background(railColor(s.phase))
			ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, railTitle(s.phase)).FontSize(12).FontWeight(600).TextColor(railColor(s.phase))
				if message := a.railDetail(); message != "" {
					ui.Text(c, message).FontSize(11).
						TextColor(textSoft).MaxLines(1)
				}
			})
		default:
			ui.Spacer(c)
		}

		if s.CanSkip() {
			a.toolbarButton(c, "跳过当前章", textSoft, s.skipChapter)
		}
		if a.railHasMenu() {
			more := ui.Button(c, "").Label("更多").FontSize(12).Height(28).Width(28).
				Padding(0).Background(ui.Transparent).Border(0, ui.Transparent)
			if more.Hovered() {
				more.Background(fill)
			}
			more.Children(func() {
				ui.Icon(c, iconMore).Size(14, 14).TextColor(textSoft)
			})
			more.PopoverMenuButton(func(m *ui.Menu) {
				if len(s.Chapters()) > 0 {
					if m.Item("查看已下载章节").Chosen() {
						a.editChapters = false
						a.showChapters = true
					}
				}
				if s.CanExport() && !s.CanStart() && s.phase != PhaseFinished {
					if m.Item("导出当前内容").Chosen() {
						a.openExportDialog()
					}
				}
				switch s.phase {
				case PhaseDownloading, PhasePaused, PhaseFailed:
					m.Separator()
					if m.Item("从头重下").Chosen() {
						s.startDownload()
					}
					if m.Item("停止并清空").Danger().Chosen() {
						a.pendingStop = true
					}
				}
			}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd})
		}
	})
}

// railHasMenu reports whether the status bar has secondary actions to fold
// into its more menu.
func (a *app) railHasMenu() bool {
	s := a.session
	if len(s.Chapters()) > 0 {
		return true
	}
	if s.CanExport() && !s.CanStart() && s.phase != PhaseFinished {
		return true
	}
	switch s.phase {
	case PhaseDownloading, PhasePaused, PhaseFailed:
		return true
	}
	return false
}

func railColor(phase Phase) ui.Color {
	switch phase {
	case PhaseFailed:
		return dangerText
	case PhasePaused:
		return warningText
	case PhaseFinished:
		return successText
	default:
		return textSoft
	}
}

// railTitle is the short state the status bar leads with, plainer than the
// phase's own name.
func railTitle(phase Phase) string {
	switch phase {
	case PhaseFinished:
		return "已导出"
	case PhaseFailed:
		return "下载中断"
	case PhasePaused:
		return "已暂停"
	case PhaseStopped:
		return "已停止"
	default:
		return phase.Title()
	}
}

// railDetail is the line under the state, without repeating it: the file name
// for a finished export, the chapter that failed, or nothing while paused,
// since the progress band already shows what was kept.
func (a *app) railDetail() string {
	s := a.session
	switch s.phase {
	case PhaseFinished:
		if saved := s.LastSaved(); saved != "" {
			return filepath.Base(saved)
		}
	case PhasePaused:
		return ""
	case PhaseFailed:
		if progress := s.Progress(); progress != nil && progress.CurrentTitle != "" {
			return progress.CurrentTitle
		}
		return ""
	}
	detail := firstLine(s.statusDetail())
	detail = strings.TrimPrefix(detail, railTitle(s.phase))
	detail = strings.TrimPrefix(detail, "EPUB 已导出：")
	detail = strings.TrimPrefix(detail, "EPUB 已导出:")
	return strings.TrimLeft(detail, "，。:： ")
}

// firstLine is the headline of a multi-line message.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func (a *app) paceLabel(snap *Progress) string {
	done := snap.Completed - a.session.startCount
	if a.session.startedAt.IsZero() || done < 2 {
		return ""
	}
	elapsed := time.Since(a.session.startedAt)
	if elapsed <= 0 {
		return ""
	}
	perMinute := float64(done) / elapsed.Minutes()
	if perMinute < 1 {
		return fmt.Sprintf("%.1f 章/分", perMinute)
	}
	return fmt.Sprintf("%.0f 章/分", perMinute)
}

func (a *app) etaLabel(snap *Progress) string {
	left := snap.Total - snap.Completed
	done := snap.Completed - a.session.startCount
	if left <= 0 || a.session.startedAt.IsZero() || done < 3 {
		return ""
	}
	elapsed := time.Since(a.session.startedAt)
	if elapsed <= 0 {
		return ""
	}
	per := elapsed / time.Duration(done)
	return shortDuration(per * time.Duration(left))
}

// showRail reports whether the run has an outcome worth a bar.
func (a *app) showRail() bool {
	switch a.session.Phase() {
	case PhaseDownloading, PhasePaused, PhaseFinished, PhaseFailed, PhaseStopped:
		return true
	}
	return len(a.session.Chapters()) > 0
}

// sheet lays a dialog out as a macOS sheet: attached to the toolbar's bottom
// edge, centred, over the window's own content with no dimming, rounded only
// at the bottom and shadowed only where it hangs free.
func (a *app) sheet(c *ui.Context, open *bool, fn func()) {
	ui.DialogBase(c, open, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.Transparent).
			Justify(ui.Start).AlignItems(ui.Center).Padding(52, 0, 0, 0)
		sheet.Padding(24).Radius(0, 0, 12, 12).Background(contentBg).
			Shadow(0, 22, 60, -18, sheetShadow)
		fn()
	})
}

// sheetTitle is the one heading every sheet starts with.
func sheetTitle(c *ui.Context, title, subtitle string) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(textMain)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(11).TextColor(textSoft)
		}
	})
}

func fieldLabel(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(textSoft)
}

// typeCover is the cover a book gets when it has no image: the first
// character set in the paper's own gray, the same at every size, so the
// toolbar, the history and the export sheet identify the book alike.
func typeCover(c *ui.Context, title string, width, height, size float32, caption string) {
	ui.Column(c).Size(width, height).Radius(4).Background(fill).
		AlignItems(ui.Center).Justify(ui.Center).Gap(6).Children(func() {
		ui.Text(c, firstRune(title)).FontSize(size).FontWeight(600).TextColor(textSoft)
		if caption != "" {
			ui.Text(c, caption).FontSize(11).TextColor(textSidebar)
		}
	})
}

// firstRune is the character a typographic cover shows.
func firstRune(title string) string {
	if runes := []rune(trimParagraph(title)); len(runes) > 0 {
		return string(runes[0])
	}
	return "书"
}

func (a *app) exportDialog(c *ui.Context) {
	a.sheet(c, &a.showExport, func() {
		ui.Column(c).Width(520).Gap(18).Children(func() {
			sheetTitle(c, "导出设置", "书名、作者和封面会写进 EPUB 的首页。")

			ui.Row(c).Gap(20).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Gap(12).Children(func() {
					ui.Column(c).Gap(5).Children(func() {
						fieldLabel(c, "作者")
						ui.TextInput(c, &a.draft.Author).Placeholder("作者").
							FontSize(13).Height(30).Padding(0, 9).Radius(6).
							Background(contentBg).Border(1, separatorStrong)
					})

					ui.Column(c).Gap(5).Children(func() {
						fieldLabel(c, "正文字体")
						fonts := make([]string, len(FontPresets))
						for i, preset := range FontPresets {
							fonts[i] = preset.DisplayName()
						}
						if a.exportFont == "" {
							a.exportFont = a.draft.BodyFont.DisplayName()
						}
						ui.Select(c, &a.exportFont, fonts).FontSize(13).Height(30).Radius(6).
							Background(fill).Border(0, ui.Transparent)
					})

					ui.Column(c).Gap(5).Children(func() {
						fieldLabel(c, "首页说明")
						ui.TextInput(c, &a.draft.FrontText).Placeholder(defaultFrontPageBodyText).
							Multiline(true).MinHeight(72).FontSize(13).Padding(8, 9).
							Radius(6).Background(contentBg).Border(1, separatorStrong)
						ui.Checkbox(c, &a.draft.ShowSource, "首页显示来源链接").FontSize(12)
					})
				})

				ui.Column(c).Width(120).Gap(8).Children(func() {
					fieldLabel(c, "封面")
					if a.coverPreview != nil {
						ui.Image(c, a.coverPreview).Size(120, 160).Radius(6).Border(1, separator)
					} else {
						typeCover(c, a.draft.Title, 120, 160, 44, "未选择封面")
					}
					label := "选择封面"
					if len(a.draft.CoverImage) > 0 {
						label = "更换封面"
					}
					ui.Button(c, label).FontSize(11).Height(26).OnClick(a.importCover)
					if len(a.draft.CoverImage) > 0 {
						remove := ui.Button(c, "移除封面").FontSize(11).Height(26).
							Background(ui.Transparent).Border(0, ui.Transparent).TextColor(textSoft)
						if remove.Hovered() {
							remove.Background(fill)
						}
						remove.OnClick(a.removeCover)
					}
				})
			})

			ui.Divider(c).Background(separator)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").FontSize(12).Height(28).Padding(0, 12).
					OnClick(func() { a.showExport = false })
				ui.PrimaryButton(c, "导出 EPUB").FontSize(12).Height(28).Padding(0, 12).
					OnClick(a.confirmExport)
			})
		})
	})
}

func (a *app) historyDialog(c *ui.Context) {
	height := float32(360 + max(0, min(len(a.historyList), 5)-3)*40)

	a.sheet(c, &a.showHistory, func() {
		ui.Column(c).Width(660).Height(height).Gap(16).Children(func() {
			sheetTitle(c, "下载历史", "每次下载都会存一份快照，可以随时重新导出。")

			ui.Row(c).Grow(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(200).Gap(1).Children(func() {
					if len(a.historyList) == 0 {
						ui.Text(c, "还没有记录").FontSize(12).TextColor(textSoft).Padding(4, 6)
						return
					}
					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).Gap(1).Children(func() {
							for _, record := range a.historyList {
								record := record
								selected := record.ID == a.selectedID
								row := ui.Row(c).Height(44).Padding(5, 8).Radius(6).Gap(8).
									AlignItems(ui.Center).OnClick(func() { a.selectHistory(record.ID) })
								switch {
								case selected:
									row.Background(accent)
								case row.Hovered():
									row.Background(fillStrong)
								}
								row.Children(func() {
									ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
										titleColor, metaColor := textMain, textSoft
										if selected {
											titleColor, metaColor = accentInk, accentInk
										}
										ui.Text(c, record.Title).FontSize(12).
											TextColor(titleColor).MaxLines(1)
										ui.Textf(c, "%s · %s", record.ProgressLabel(), record.UpdatedAt.Format("01-02")).
											FontSize(11).TextColor(metaColor).FontFeatures("tnum")
									})
								})
							}
						})
					})
				})

				ui.Box(c).Width(1).Shrink(0).Background(separator)

				ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
					record, ok := a.selectedRecord()
					if !ok {
						ui.Spacer(c)
						ui.Text(c, "还没有下载历史").FontSize(12).TextColor(textSoft).TextAlign(ui.Center)
						ui.Spacer(c)
						return
					}

					ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
						if a.historyCover != nil {
							ui.Image(c, a.historyCover).Size(64, 85).Radius(6).Border(1, separator)
						} else {
							typeCover(c, record.Title, 64, 85, 28, "")
						}
						ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
							ui.Text(c, record.Title).FontSize(15).FontWeight(600).
								TextColor(textMain).MaxLines(2)
							if record.Author != "" && record.Author != "Unknown" {
								ui.Text(c, record.Author).FontSize(12).TextColor(textSoft)
							}
							ui.Textf(c, "%s · %s", completeLabel(record), record.SourceHost()).
								FontSize(11).TextColor(textSoft)
						})
					})

					ui.Column(c).Gap(6).Children(func() {
						detailRow(c, "进度", record.ProgressLabel())
						detailRow(c, "更新", record.UpdatedAt.Format("2006-01-02 15:04"))
						detailRow(c, "创建", record.CreatedAt.Format("2006-01-02 15:04"))
					})

					ui.Text(c, historyMessage(record)).FontSize(11).LineHeight(1.6).TextColor(textSoft)
					if a.historyError != "" {
						ui.Text(c, a.historyError).FontSize(11).TextColor(dangerText)
					}
				})
			})

			ui.Divider(c).Background(separator)

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "关闭").FontSize(12).Height(28).Padding(0, 12).
					OnClick(func() { a.showHistory = false })
				b := ui.Button(c, "删除记录").FontSize(12).Height(28).Padding(0, 12).
					TextColor(dangerText).Disabled(a.selectedID == "")
				if b.Clicked() {
					a.pendingDelete = true
				}
				ui.Spacer(c)
				_, ok := a.selectedRecord()
				ui.PrimaryButton(c, "导出 EPUB").FontSize(12).Height(28).Padding(0, 12).
					Disabled(!ok).OnClick(a.exportHistory)
			})
		})
	})
}

func completeLabel(record HistoryRecord) string {
	if record.Complete() {
		return "完整 EPUB"
	}
	return "部分 EPUB"
}

func historyMessage(record HistoryRecord) string {
	if len(record.Chapters) == 0 {
		return "这条记录还没有保存任何章节内容，因此暂时不能导出文件。"
	}
	if !record.Complete() {
		return "此记录保存了当时已下载的内容，导出时只包含这些章节。"
	}
	return "此记录保存了完整下载内容，你可以直接再次导出文件。"
}

func detailRow(c *ui.Context, title, value string) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(11).TextColor(textSoft).Width(36)
		ui.Text(c, value).FontSize(12).TextColor(textMain).Grow(1)
	})
}

func (a *app) chaptersDialog(c *ui.Context) {
	chapters := a.session.Chapters()
	height := float32(210 + min(len(chapters), 6)*33)

	a.sheet(c, &a.showChapters, func() {
		ui.Column(c).Width(460).Height(height).Gap(14).Children(func() {
			sheetTitle(c, "已下载章节",
				fmt.Sprintf("共 %d 章 · 删除只影响导出，不回退下载进度", len(chapters)))

			ui.List(c, nil, len(chapters), func(i int) {
				chapter := chapters[i]
				ui.Row(c).Padding(0, 4).Height(32).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, chapter.Title).FontSize(13).TextColor(textMain).Grow(1).MaxLines(1)
					if a.editChapters && !a.session.Busy() {
						remove := ui.Button(c, "移除").FontSize(11).Height(24).Padding(0, 8).
							Background(ui.Transparent).Border(0, ui.Transparent).TextColor(dangerText)
						if remove.Hovered() {
							remove.Background(fill)
						}
						if remove.Clicked() {
							a.session.removeChapter(chapter.Order)
						}
					}
				})
				ui.Divider(c).Background(separator)
			}).Grow(1)

			ui.Divider(c).Background(separator)

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if len(chapters) > 0 && !a.session.Busy() {
					label := "编辑"
					if a.editChapters {
						label = "完成"
					}
					ui.Button(c, label).FontSize(12).Height(28).Padding(0, 12).
						OnClick(func() { a.editChapters = !a.editChapters })
				}
				ui.Spacer(c)
				ui.Button(c, "关闭").FontSize(12).Height(28).Padding(0, 12).
					OnClick(func() { a.showChapters = false; a.editChapters = false })
			})
		})
	})
}

func (a *app) stopAlert(c *ui.Context) {
	a.sheet(c, &a.pendingStop, func() {
		ui.Column(c).Width(380).Gap(16).Children(func() {
			sheetTitle(c, "停止并清空这次下载？",
				"已下载的章节会保留在下载历史里，当前进度会清空。")
			ui.Divider(c).Background(separator)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").FontSize(12).Height(28).Padding(0, 12).
					OnClick(func() { a.pendingStop = false })
				ui.Button(c, "停止并清空").FontSize(12).Height(28).Padding(0, 12).
					TextColor(dangerText).OnClick(func() {
					a.session.stopDownload()
					a.pendingStop = false
				})
			})
		})
	})
}

func (a *app) deleteAlert(c *ui.Context) {
	a.sheet(c, &a.pendingDelete, func() {
		ui.Column(c).Width(420).Gap(16).Children(func() {
			sheetTitle(c, "删除这条历史记录？",
				"这会删除应用内保存的历史快照，不会删除你已经导出的本地 EPUB 文件。")
			ui.Divider(c).Background(separator)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").FontSize(12).Height(28).Padding(0, 12).
					OnClick(func() { a.pendingDelete = false })
				ui.Button(c, "删除").FontSize(12).Height(28).Padding(0, 12).
					TextColor(dangerText).OnClick(func() {
					a.deleteHistory()
					a.pendingDelete = false
				})
			})
		})
	})
}
