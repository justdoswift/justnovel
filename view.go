package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The visual system, measured off Rex: a dark workspace with a near-black
// canvas, panels a shade lighter, and the work lifted by value rather than by
// borders. The accent is the system blue, spent only on the one action to take
// next and on the row that is moving.
var (
	canvas     = ui.Hex("#0a0d12")
	railTone   = ui.Hex("#0e1117")
	card       = ui.Hex("#111419")
	cardHover  = ui.Hex("#161a20")
	cardSunk   = ui.Hex("#1b2027")
	textMain   = ui.Hex("#e7edf3")
	textSoft   = ui.Hex("#a8b2bf")
	textFaint  = ui.Hex("#7c8794")
	textMuted  = ui.Hex("#5c6673")
	hairline   = ui.RGBA(255, 255, 255, 0.06)
	hairlineUp = ui.RGBA(255, 255, 255, 0.11)

	accent     = ui.Hex("#4582f9")
	accentInk  = ui.Hex("#ffffff")
	accentDim  = ui.RGBA(69, 130, 249, 0.14)
	danger     = ui.Hex("#f06a6a")
	warning    = ui.Hex("#e0a33e")
	success    = ui.Hex("#5fbf7f")
	progressBg = ui.Hex("#1b2027")

	cardRing   = ui.RGBA(255, 255, 255, 0.07)
	cardShadow = ui.RGBA(0, 0, 0, 0.45)
)

var theme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = canvas
	t.Surface = card
	t.SurfaceHover = cardHover
	t.SurfacePressed = cardSunk
	t.Border = hairlineUp
	t.Text = textMain
	t.TextMuted = textFaint
	t.Accent = accent
	t.AccentHover = ui.Hex("#5c92fa")
	t.AccentPressed = ui.Hex("#3570e0")
	t.AccentText = accentInk
	t.Danger = danger
	t.Warning = warning
	t.Success = success
	t.Selection = ui.RGBA(69, 130, 249, 0.28)
	t.FocusRingWidth = 2
	t.FocusRingOffset = 1
	t.Radius = 8
	t.Spacing = 4
	t.FontSize = 13
	return t
}()

var (
	iconBack   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M10.5 3.5 6 8l4.5 4.5"/></svg>`))
	iconReload = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 8a5 5 0 1 1-1.5-3.6"/><path d="M13 3v3.5H9.5"/></svg>`))
)

// view draws the window: a title bar with the source and the one action, the
// work in the middle, and a status line along the bottom.
func (a *app) view(c *ui.Context) {
	c.SetTheme(theme)

	ui.Column(c).Fill().Background(canvas).Children(func() {
		a.topBar(c)
		ui.Row(c).Grow(1).Children(func() {
			a.stage(c)
		})
		a.statusBar(c)
	})

	a.exportDialog(c)
	a.historyDialog(c)
	a.chaptersDialog(c)
	a.stopAlert(c)
	a.deleteAlert(c)
}

// topBar is the window's title bar: the source, the search field, and the one
// action that moves the work forward.
func (a *app) topBar(c *ui.Context) {
	ui.Column(c).Children(func() {
		ui.Row(c).Height(54).Padding(0, 16, 0, 84).Gap(10).AlignItems(ui.Center).Children(func() {
			a.brand(c)
			ui.Spacer(c)
			a.topActionButton(c)
			a.historyAction(c)
		})

		ui.Divider(c).Background(hairline)
	})
}

func (a *app) brand(c *ui.Context) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Width(112).DragWindow().Children(func() {
		ui.Column(c).Size(22, 22).Radius(5).Background(textMain).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "J").FontSize(12).Bold().TextColor(card)
		})
		ui.Text(c, "justnovel").FontSize(13).Bold().LetterSpacing(-0.1).TextColor(textMain)
	})
}

// sourcePicker is the first thing the user picks: which site to look in.
func (a *app) sourcePicker(c *ui.Context) {
	names := make([]string, len(Sources))
	for i, source := range Sources {
		names[i] = source.Name
	}
	chosen := a.source.Name
	ui.Select(c, &chosen, names).FontSize(12).Height(32).Width(112).
		Background(cardSunk).Radius(8).OnChange(func() {
		for _, source := range Sources {
			if source.Name == chosen {
				a.selectSource(source.Kind)
			}
		}
	})
}

// searchField is how a book is found: type a title, press Enter.
func (a *app) searchField(c *ui.Context) {
	input := ui.TextInput(c, &a.query).
		Placeholder("搜索书名，回车开始").
		FontSize(12).Height(32).Grow(1).MaxWidth(420).
		Background(cardSunk).Radius(8).
		PlaceholderColor(textMuted)
	if input.Submitted() {
		a.search()
		a.searchWithTimeout()
	}
}

func (a *app) historyAction(c *ui.Context) {
	ui.Row(c).Gap(6).Height(32).Padding(0, 10).Radius(8).AlignItems(ui.Center).
		OnClick(a.openHistory).Children(func() {
		ui.Text(c, "历史").FontSize(12).TextColor(textSoft)
		if a.history != nil {
			if count := a.history.Count(); count > 0 {
				ui.Text(c, itoa(count)).FontSize(11).TextColor(textFaint)
			}
		}
	})
}

// topAction is the one primary action, named for the next step.
func (a *app) topAction() (string, bool, func()) {
	switch {
	case a.canPause():
		return "暂停下载", true, a.pauseDownload
	case a.job.Phase == PhaseFailed && a.catalog != nil && a.next < len(a.catalog.Chapters):
		return "重试当前章", true, a.resumeDownload
	case a.canResume():
		return "继续下载", true, a.resumeDownload
	case a.job.Phase == PhaseFinished && len(a.chapters) > 0:
		return "导出 EPUB", true, a.openExportDialog
	case a.job.Phase == PhaseFailed && len(a.chapters) > 0 && a.catalog != nil && a.next >= len(a.catalog.Chapters):
		return "导出 EPUB", true, a.openExportDialog
	case a.catalog != nil && !a.job.Busy():
		return "开始下载", true, a.startDownload
	case a.job.Busy():
		return "处理中…", false, nil
	default:
		// Nothing to do from the bar: the middle of the window holds the
		// search, and the bar stays quiet until a run starts.
		return "", false, nil
	}
}

func (a *app) topActionButton(c *ui.Context) {
	label, enabled, action := a.topAction()
	if label == "" {
		return
	}
	if enabled {
		ui.PrimaryButton(c, label).FontSize(12).Height(32).Width(96).OnClick(action)
		return
	}
	ui.Button(c, label).FontSize(12).Height(32).Width(96).Disabled(true)
}

// stage is the work in the middle: the search results, the book, or the run.
func (a *app) stage(c *ui.Context) {
	ui.Column(c).Grow(1).FillHeight().Padding(0, 16, 0, 16).Children(func() {
		switch {
		case a.searching:
			a.searchingStage(c)
		case len(a.hits) > 0:
			a.resultsStage(c)
		case a.catalog != nil:
			a.bookStage(c)
		default:
			a.emptyStage(c)
		}
		a.hiddenBrowser(c)
	})
}

// hiddenBrowser keeps the webview in the tree at all times. Searching and
// reading a catalog both go through it: it loads the page, runs the site's
// script, and reports back. A webview that is not in the tree never loads
// anything, so the search would wait forever.
func (a *app) hiddenBrowser(c *ui.Context) {
	if a.session == nil || a.session.view == nil {
		return
	}
	if a.showBrowser {
		return // the panel below draws it
	}
	ui.Box(c).Absolute().Top(0).Left(0).Size(2, 2).Opacity(0.01).Children(func() {
		ui.WebView(c, a.session.view).Size(2, 2)
	})
}

// card is the surface the work sits on.
func (a *app) card(c *ui.Context, body func()) {
	ui.Column(c).Grow(1).MinHeight(0).Radius(12).Background(card).
		Border(1, cardRing).Shadow(0, 1, 3, 0, cardShadow).
		Shadow(0, 8, 20, -6, cardShadow).
		ClipInset(0, 0, 0, 0, 12).Children(body)
}

// emptyStage is the first screen: one search row, then the books already
// downloaded. The row is a single line of equal-height controls, so it reads
// as one instrument rather than a field with two loose buttons under it.
func (a *app) emptyStage(c *ui.Context) {
	var records []HistoryRecord
	if a.history != nil {
		records = a.history.List()
	}

	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(640).Gap(20).Children(func() {
			ui.Column(c).Gap(7).Children(func() {
				ui.Text(c, "搜一本书").FontSize(26).Bold().
					TextColor(textMain).LetterSpacing(-0.6)
				ui.Text(c, "选站点，输入书名，回车。搜到后点一下就开始下载。").FontSize(12).
					LineHeight(1.7).TextColor(textSoft)
			})

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				a.sourcePicker(c)
				input := ui.TextInput(c, &a.query).
					Placeholder("输入书名").
					FontSize(13).Height(38).Grow(1).Padding(0, 13).
					Background(card).Radius(9).
					PlaceholderColor(textMuted)
				if input.Submitted() {
					a.search()
					a.searchWithTimeout()
				}
				ui.PrimaryButton(c, "搜索").FontSize(12).Height(38).Width(84).OnClick(func() {
					a.search()
					a.searchWithTimeout()
				})
			})

			if a.searchError != "" {
				ui.Text(c, a.searchError).FontSize(12).TextColor(danger).MaxLines(3)
			}

			if len(records) > 0 {
				a.recentList(c, records)
			}
		})
	})
}

// recentList is the books already downloaded: a way back to earlier work, and
// something real under the search so the first screen is not a void.
func (a *app) recentList(c *ui.Context, records []HistoryRecord) {
	ui.Column(c).Gap(2).Children(func() {
		ui.Row(c).Height(30).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "最近下载").FontSize(10).TextColor(textMuted)
			ui.Spacer(c)
			ui.Textf(c, "%d 本", len(records)).FontSize(10).TextColor(textMuted)
		})
		for i, record := range records {
			if i >= 6 {
				break
			}
			record := record
			ui.Row(c).Height(38).Padding(8, 0).Radius(7).Gap(12).AlignItems(ui.Center).
				OnClick(a.openHistory).Children(func() {
				ui.Text(c, record.Title).FontSize(12).TextColor(textSoft).Grow(1).MaxLines(1)
				ui.Text(c, record.ProgressLabel()).FontSize(10).
					TextColor(textMuted).Width(60).FontFeatures("tnum")
				ui.Text(c, record.UpdatedAt.Format("01-02")).FontSize(10).
					TextColor(textMuted).Width(40).FontFeatures("tnum")
			})
		}
	})
}

// searchingStage shows the site while it answers. The panel is not decoration:
// 69shuba sits behind Cloudflare, so the page has to be visible for the check
// to be solvable, and the results arrive in it.
func (a *app) searchingStage(c *ui.Context) {
	a.card(c, func() {
		ui.Row(c).Height(44).Padding(0, 18).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "正在 %s 搜索「%s」", a.source.Name, a.query).
				FontSize(13).Bold().TextColor(textMain)
			ui.Spacer(c)
			ui.Text(c, "站点要验证时，在下面完成").FontSize(11).TextColor(textFaint)
		})
		ui.Divider(c).Background(hairline)

		if a.session != nil && a.session.view != nil {
			ui.WebView(c, a.session.view).Grow(1).MinHeight(0)
			return
		}
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, "正在加载站点页面…").FontSize(13).TextColor(textSoft)
		})
	})
}

// resultsStage is what the search found: pick one, and it opens. The list
// hugs its rows instead of stretching to the window, so three results do not
// leave a page of empty card under them.
func (a *app) resultsStage(c *ui.Context) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Padding(48, 0, 0, 0).Children(func() {
		ui.Column(c).Width(680).Gap(0).Radius(12).Background(card).
			Border(1, cardRing).Shadow(0, 1, 3, 0, cardShadow).
			Shadow(0, 8, 20, -6, cardShadow).Children(func() {
			ui.Row(c).Height(44).Padding(0, 18).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Textf(c, "「%s」的结果", a.query).FontSize(13).Bold().TextColor(textMain)
				ui.Textf(c, "%d 本", len(a.hits)).FontSize(11).TextColor(textFaint)
				ui.Spacer(c)
				ui.Text(c, "换个词").FontSize(11).TextColor(textSoft).
					Padding(4, 8).OnClick(a.cancelSearch)
			})
			ui.Divider(c).Background(hairline)

			for _, hit := range a.hits {
				hit := hit
				ui.Row(c).Height(52).Padding(0, 18).Gap(12).AlignItems(ui.Center).
					OnClick(func() { a.pickHit(hit) }).Children(func() {
					ui.Column(c).Gap(2).Grow(1).Children(func() {
						ui.Text(c, hit.Title).FontSize(13).TextColor(textMain).MaxLines(1)
						if hit.Author != "" {
							ui.Text(c, hit.Author).FontSize(11).TextColor(textFaint).MaxLines(1)
						}
					})
					ui.Text(c, "打开").FontSize(11).TextColor(accent)
				})
				ui.Divider(c).Background(hairline)
			}
		})
	})
}

// bookStage is the book once its catalog is read: start it, and watch it run.
func (a *app) bookStage(c *ui.Context) {
	a.card(c, func() {
		a.bookHeader(c)
		ui.Divider(c).Background(hairline)

		if len(a.log) == 0 {
			a.catalogPreview(c)
			return
		}
		ui.List(c, &a.logState, len(a.log), func(i int) {
			logRow(c, a.log[i])
		}).Grow(1).MinHeight(0)
		a.logLive(c)

		if a.showRail() {
			ui.Divider(c).Background(hairline)
			a.rail(c)
		}
	})
}

// bookHeader is the book's name and how far the run has got.
func (a *app) bookHeader(c *ui.Context) {
	ui.Column(c).Gap(10).Padding(16, 20, 14, 20).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, a.catalog.Title).FontSize(15).Bold().
					TextColor(textMain).MaxLines(1)
				meta := fmt.Sprintf("%d 章", len(a.catalog.Chapters))
				if a.catalog.Author != "" && a.catalog.Author != "Unknown" {
					meta = a.catalog.Author + " · " + meta
				}
				ui.Text(c, meta).FontSize(11).TextColor(textFaint).MaxLines(1)
			})
			if a.session.view != nil && a.session.State().Loaded {
				label := "显示浏览器"
				if a.showBrowser {
					label = "收起浏览器"
				}
				ui.Button(c, label).FontSize(11).OnClick(func() { a.showBrowser = !a.showBrowser })
			}
		})

		if snap := a.job.Snap; snap != nil {
			ui.Row(c).AlignItems(ui.Baseline).Gap(5).Children(func() {
				ui.Text(c, itoa(snap.Completed)).FontSize(26).Bold().
					TextColor(textMain).FontFeatures("tnum").LetterSpacing(-1)
				ui.Text(c, "/"+itoa(snap.Total)).FontSize(12).
					TextColor(textFaint).FontFeatures("tnum")
				ui.Spacer(c)
				if pace := a.paceLabel(snap); pace != "" {
					ui.Text(c, pace).FontSize(11).TextColor(textFaint).FontFeatures("tnum")
				}
				if eta := a.etaLabel(snap); eta != "" {
					ui.Text(c, "· 约 "+eta).FontSize(11).TextColor(textFaint).FontFeatures("tnum")
				}
				ui.Textf(c, "%d%%", int(snap.Fraction()*100+0.5)).FontSize(12).
					TextColor(accent).FontFeatures("tnum")
			})
			ui.Progress(c, snap.Fraction()).Height(4).TextColor(accent).Background(progressBg)
		}
	})
}

// catalogPreview is the book's contents before a run.
func (a *app) catalogPreview(c *ui.Context) {
	chapters := a.catalog.Chapters
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Row(c).Height(32).Padding(0, 20).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "章节").FontSize(10).TextColor(textMuted)
			ui.Spacer(c)
			ui.Textf(c, "共 %d 章", len(chapters)).FontSize(10).
				TextColor(textMuted).FontFeatures("tnum")
		})
		ui.List(c, nil, len(chapters), func(i int) {
			chapter := chapters[i]
			ui.Row(c).Height(26).Padding(0, 20).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Textf(c, "%03d", chapter.Order).FontSize(10).
					TextColor(textMuted).Width(28).FontFeatures("tnum")
				ui.Text(c, chapter.Title).FontSize(11).TextColor(textSoft).Grow(1).MaxLines(1)
			})
		}).Grow(1).MinHeight(0)
	})
}

// logLive is the chapter the crawler is on right now.
func (a *app) logLive(c *ui.Context) {
	snap := a.job.Snap
	if snap == nil || !a.running {
		return
	}
	ui.Divider(c).Background(hairline)
	ui.Row(c).Height(30).Padding(0, 20).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, time.Now().Format("15:04:05")).FontSize(10).
			TextColor(textMuted).Width(56).FontFeatures("tnum")
		ui.Text(c, "▸").FontSize(10).TextColor(accent).Width(12)
		ui.Textf(c, "%03d  %s", snap.CurrentIndex, snap.CurrentTitle).
			FontSize(11).TextColor(textMain).Grow(1).MaxLines(1)
		if snap.Transport == TransportBrowser {
			ui.Text(c, "浏览器").FontSize(10).TextColor(warning)
		}
	})
}

// logRow is one line of the run: when, what, and how long it took.
func logRow(c *ui.Context, line LogLine) {
	mark, markColor := "·", textMuted
	textColor := textSoft
	switch line.Kind {
	case LogChapter:
		mark, markColor, textColor = "✓", success, textSoft
	case LogRetry:
		mark, markColor, textColor = "↻", warning, warning
	case LogFallback:
		mark, markColor, textColor = "⇄", warning, textMain
	case LogWarn:
		mark, markColor, textColor = "‖", warning, warning
	case LogError:
		mark, markColor, textColor = "!", danger, danger
	case LogDone:
		mark, markColor, textColor = "✓", success, textMain
	case LogInfo:
		mark, markColor, textColor = "›", textFaint, textMain
	}

	ui.Row(c).Height(23).Padding(0, 20).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, line.At.Format("15:04:05")).FontSize(10).
			TextColor(textMuted).Width(56).FontFeatures("tnum")
		ui.Text(c, mark).FontSize(10).TextColor(markColor).Width(12)
		ui.Text(c, line.Text).FontSize(11).TextColor(textColor).Grow(1).MaxLines(1)
		if line.Detail != "" {
			ui.Text(c, line.Detail).FontSize(10).TextColor(warning).MaxLines(1)
		}
		if line.Took > 0 {
			ui.Text(c, shortDuration(line.Took)).FontSize(10).
				TextColor(textMuted).Width(40).TextAlign(ui.End).FontFeatures("tnum")
		}
	})
}

// rail is the run's outcome and what can be done about it.
func (a *app) rail(c *ui.Context) {
	showStatus := a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed || a.job.Phase == PhaseFinished
	ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(10, 20).Children(func() {
		if showStatus {
			ui.Box(c).Size(6, 6).Radius(3).Background(railColor(a.job.Phase))
			ui.Column(c).Gap(1).Grow(1).Children(func() {
				ui.Text(c, a.railTitle()).FontSize(12).Bold().TextColor(railColor(a.job.Phase))
				if message := a.railMessage(); message != "" {
					ui.Text(c, message).FontSize(11).TextColor(textFaint).MaxLines(2)
				}
			})
		} else {
			ui.Spacer(c)
		}

		if a.job.Phase == PhaseFailed && !a.running && a.catalog != nil && a.next < len(a.catalog.Chapters) {
			ui.Button(c, "跳过当前章").FontSize(11).OnClick(a.skipChapter)
			ui.Button(c, "从头重下").FontSize(11).OnClick(a.restartDownload)
		}
		if a.canExportSession() {
			ui.Button(c, "导出当前内容").FontSize(11).OnClick(a.openExportDialog)
		}
		if len(a.chapters) > 0 {
			ui.Button(c, "查看已下载章节").FontSize(11).OnClick(func() { a.showChapters = true })
		}
		if a.running || a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed {
			ui.Text(c, "停止并清空").FontSize(11).TextColor(textMuted).
				Padding(4, 8).OnClick(func() { a.pendingStop = true })
		}
	})
}

func railColor(phase JobPhase) ui.Color {
	switch phase {
	case PhaseFailed:
		return danger
	case PhasePaused:
		return warning
	case PhaseFinished:
		return success
	default:
		return textSoft
	}
}

func (a *app) railTitle() string {
	switch a.job.Phase {
	case PhaseFailed:
		return "需要处理"
	case PhasePaused:
		return "下载已暂停"
	case PhaseFinished:
		return "下载已完成"
	default:
		return "当前会话"
	}
}

func (a *app) railMessage() string {
	switch a.job.Phase {
	case PhaseFailed:
		if snap := a.job.Snap; snap != nil && snap.Completed > 0 {
			return fmt.Sprintf("已完成 %d / %d 章，可继续、跳过当前章，或先导出当前内容。", snap.Completed, snap.Total)
		}
		if len(a.chapters) > 0 && a.catalog != nil {
			return fmt.Sprintf("已完成 %d / %d 章，可继续、跳过当前章，或先导出当前内容。",
				len(a.chapters), len(a.catalog.Chapters))
		}
		return "当前还没有成功章节，可以继续、跳过当前章或重新开始整次下载。"
	case PhasePaused:
		if snap := a.job.Snap; snap != nil {
			return fmt.Sprintf("当前会话已保留 %d / %d 章。", snap.Completed, snap.Total)
		}
		return "当前会话已保留。"
	case PhaseFinished:
		return a.job.Message
	default:
		return ""
	}
}

// statusBar closes the window along its bottom edge.
func (a *app) statusBar(c *ui.Context) {
	state := a.session.State()
	ui.Row(c).Height(28).Padding(0, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		chip := func(label, value string) {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(10).TextColor(textMuted)
				ui.Text(c, value).FontSize(10).TextColor(textFaint).FontFeatures("tnum")
			})
		}
		chip("站点", a.source.Name)
		chip("会话", itoa(state.Cookies))
		if a.history != nil {
			chip("缓存", humanBytes(a.history.ByteCount()))
		}
		ui.Spacer(c)
		ui.Text(c, "仅抓取你有权访问的公开页面").FontSize(10).TextColor(textMuted)
	})
}

// reading is one small labelled figure.
func (a *app) reading(c *ui.Context, label, value string) {
	ui.Row(c).Gap(5).AlignItems(ui.Baseline).Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(textMuted).SingleLine()
		ui.Text(c, value).FontSize(11).TextColor(textSoft).FontFeatures("tnum").SingleLine()
	})
}

func (a *app) paceLabel(snap *Progress) string {
	done := snap.Completed - a.startCount
	if a.startedAt.IsZero() || done < 2 {
		return ""
	}
	elapsed := time.Since(a.startedAt)
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
	done := snap.Completed - a.startCount
	if left <= 0 || a.startedAt.IsZero() || done < 3 {
		return ""
	}
	elapsed := time.Since(a.startedAt)
	if elapsed <= 0 {
		return ""
	}
	per := elapsed / time.Duration(done)
	return shortDuration(per * time.Duration(left))
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()+0.5))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()+0.5))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) - hours*60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// sectionLabel frames a block of a dialog.
func sectionLabel(c *ui.Context, label string) {
	ui.Text(c, label).FontSize(11).TextColor(textFaint)
}

func firstRune(title string) string {
	if runes := []rune(trimParagraph(title)); len(runes) > 0 {
		return string(runes[0])
	}
	return "书"
}

func (a *app) exportDialog(c *ui.Context) {
	ui.DialogBase(c, &a.showExport, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		sheet.Padding(20).Radius(12).Background(card).Border(1, cardRing).
			Shadow(0, 24, 60, -8, ui.RGBA(0, 0, 0, 0.55))

		ui.Column(c).Width(600).Gap(16).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "导出设置").FontSize(15).Bold().TextColor(textMain)
				ui.Text(c, "书名、作者和封面会写进 EPUB 的首页。").FontSize(11).TextColor(textFaint)
			})

			ui.Row(c).Gap(22).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Gap(14).Children(func() {
					ui.Column(c).Gap(5).Children(func() {
						sectionLabel(c, "作者")
						ui.TextInput(c, &a.draft.Author).Placeholder("作者").FontSize(12).Height(28)
					})

					ui.Column(c).Gap(5).Children(func() {
						sectionLabel(c, "正文字体")
						fonts := make([]string, len(FontPresets))
						for i, preset := range FontPresets {
							fonts[i] = preset.DisplayName()
						}
						if a.exportFont == "" {
							a.exportFont = a.draft.BodyFont.DisplayName()
						}
						ui.Select(c, &a.exportFont, fonts).FontSize(12).Height(28)
					})

					ui.Column(c).Gap(5).Children(func() {
						sectionLabel(c, "首页说明")
						ui.TextInput(c, &a.draft.FrontText).Placeholder(defaultFrontPageBodyText).
							Multiline(true).MinHeight(60).FontSize(12)
						ui.Checkbox(c, &a.draft.ShowSource, "首页显示来源链接").FontSize(11)
					})
				})

				ui.Column(c).Width(140).Gap(7).Children(func() {
					sectionLabel(c, "封面")
					if a.coverPreview != nil {
						ui.Image(c, a.coverPreview).Size(140, 187).Radius(6).Border(1, cardRing)
					} else {
						ui.Column(c).Size(140, 187).Radius(6).Background(cardSunk).
							AlignItems(ui.Center).Justify(ui.Center).Gap(5).Children(func() {
							ui.Text(c, firstRune(a.draft.Title)).FontSize(30).Bold().TextColor(textMuted)
							ui.Text(c, "未选择封面").FontSize(10).TextColor(textMuted)
						})
					}
					label := "选择封面"
					if len(a.draft.CoverImage) > 0 {
						label = "更换封面"
					}
					ui.Button(c, label).FontSize(11).OnClick(a.importCover)
					if len(a.draft.CoverImage) > 0 {
						ui.Button(c, "移除封面").FontSize(11).OnClick(a.removeCover)
					}
				})
			})

			ui.Divider(c).Background(hairline)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").FontSize(11).OnClick(func() { a.showExport = false })
				ui.PrimaryButton(c, "导出 EPUB").FontSize(11).OnClick(a.confirmExport)
			})
		})
	})
}

func (a *app) historyDialog(c *ui.Context) {
	height := float32(180 + min(len(a.historyList), 5)*40)
	ui.DialogBase(c, &a.showHistory, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		sheet.Padding(20).Radius(12).Background(card).Border(1, cardRing).
			Shadow(0, 24, 60, -8, ui.RGBA(0, 0, 0, 0.55))

		ui.Column(c).Width(720).Height(height).Gap(14).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "下载历史").FontSize(15).Bold().TextColor(textMain)
				ui.Text(c, "每次下载都会存一份快照，可以随时重新导出。").FontSize(11).TextColor(textFaint)
			})

			ui.Row(c).Grow(1).Gap(18).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(220).Gap(1).Children(func() {
					if len(a.historyList) == 0 {
						ui.Text(c, "还没有记录").FontSize(12).TextColor(textSoft)
						ui.Text(c, "开始一次下载后，这里会保存可再次导出的 EPUB 快照。").FontSize(11).TextColor(textFaint)
						return
					}
					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).Gap(1).Children(func() {
							for _, record := range a.historyList {
								selected := record.ID == a.selectedID
								row := ui.Row(c).Height(44).Padding(8, 0).Radius(7).Gap(8).AlignItems(ui.Center).Children(func() {
									ui.Column(c).Gap(1).Grow(1).Children(func() {
										color := textSoft
										if selected {
											color = textMain
										}
										ui.Text(c, record.Title).FontSize(12).TextColor(color).MaxLines(1)
										ui.Textf(c, "%s · %s", record.ProgressLabel(), record.UpdatedAt.Format("01-02")).
											FontSize(10).TextColor(textMuted)
									})
								}).OnClick(func() { a.selectHistory(record.ID) })
								if selected {
									row.Background(cardSunk)
								}
							}
						})
					})
				})

				ui.Divider(c).Background(hairline)

				ui.Column(c).Grow(1).Gap(10).Children(func() {
					record, ok := a.selectedRecord()
					if !ok {
						ui.Spacer(c)
						ui.Text(c, "还没有下载历史").FontSize(12).TextColor(textSoft).TextAlign(ui.Center)
						ui.Spacer(c)
						return
					}

					ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
						if a.historyCover != nil {
							ui.Image(c, a.historyCover).Size(84, 112).Radius(5).Border(1, cardRing)
						}
						ui.Column(c).Gap(3).Grow(1).Children(func() {
							ui.Text(c, record.Title).FontSize(14).Bold().TextColor(textMain).MaxLines(2)
							if record.Author != "" && record.Author != "Unknown" {
								ui.Text(c, record.Author).FontSize(11).TextColor(textSoft)
							}
							ui.Textf(c, "%s · %s", completeLabel(record), record.SourceHost()).
								FontSize(10).TextColor(accent)
						})
					})

					ui.Column(c).Gap(5).Children(func() {
						detailRow(c, "进度", record.ProgressLabel())
						detailRow(c, "更新", record.UpdatedAt.Format("2006-01-02 15:04"))
						detailRow(c, "创建", record.CreatedAt.Format("2006-01-02 15:04"))
					})

					ui.Text(c, historyMessage(record)).FontSize(11).LineHeight(1.6).TextColor(textSoft)
					if a.historyError != "" {
						ui.Text(c, a.historyError).FontSize(11).TextColor(danger)
					}
				})
			})

			ui.Divider(c).Background(hairline)

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "关闭").FontSize(11).OnClick(func() { a.showHistory = false })
				ui.Button(c, "删除记录").FontSize(11).TextColor(danger).Disabled(a.selectedID == "").
					OnClick(func() { a.pendingDelete = true })
				ui.Spacer(c)
				_, ok := a.selectedRecord()
				ui.PrimaryButton(c, "导出 EPUB").FontSize(11).Disabled(!ok).OnClick(a.exportHistory)
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
		ui.Text(c, title).FontSize(10).TextColor(textMuted).Width(36)
		ui.Text(c, value).FontSize(11).TextColor(textMain).Grow(1)
	})
}

func (a *app) chaptersDialog(c *ui.Context) {
	height := float32(170 + min(len(a.chapters), 6)*34)
	ui.DialogBase(c, &a.showChapters, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		sheet.Padding(20).Radius(12).Background(card).Border(1, cardRing).
			Shadow(0, 24, 60, -8, ui.RGBA(0, 0, 0, 0.55))

		ui.Column(c).Width(520).Height(height).Gap(12).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "已下载章节").FontSize(15).Bold().TextColor(textMain)
				ui.Text(c, "删除只影响导出，不回退下载进度").FontSize(11).TextColor(textFaint)
			})

			ui.Textf(c, "%d 章", len(a.chapters)).FontSize(11).TextColor(textMuted)

			ui.List(c, nil, len(a.chapters), func(i int) {
				chapter := a.chapters[i]
				ui.Row(c).Padding(6, 0).Height(32).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Textf(c, "%03d", chapter.Order).FontSize(10).
						TextColor(textMuted).Width(28).FontFeatures("tnum")
					ui.Text(c, chapter.Title).FontSize(12).TextColor(textMain).Grow(1).MaxLines(1)
					if !a.job.Busy() {
						ui.Text(c, "移除").FontSize(11).TextColor(textSoft).
							Padding(4, 6).OnClick(func() { a.removeChapter(chapter.Order) })
					}
				})
				ui.Divider(c).Background(hairline)
			}).Grow(1)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "关闭").FontSize(11).OnClick(func() { a.showChapters = false })
			})
		})
	})
}

func (a *app) stopAlert(c *ui.Context) {
	switch ui.AlertDialog(c, &a.pendingStop, "停止并清空这次下载？",
		"已下载的章节会保留在下载历史里，当前进度会清空。", "取消", "停止并清空") {
	case 1:
		a.stopDownload()
	}
}

func (a *app) deleteAlert(c *ui.Context) {
	switch ui.AlertDialog(c, &a.pendingDelete, "删除这条历史记录？",
		"这会删除应用内保存的历史快照，不会删除你已经导出的本地 EPUB 文件。", "取消", "删除记录") {
	case 1:
		a.deleteHistory()
	}
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
		return fmt.Sprintf("%d %s", n, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// showRail reports whether the run has an outcome worth a bar.
func (a *app) showRail() bool {
	switch a.job.Phase {
	case PhasePaused, PhaseFinished, PhaseFailed:
		return true
	}
	return len(a.chapters) > 0
}

var _ = filepath.Base
var _ = strings.TrimSpace
