package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The app's visual system: a graphite console for a scraping job, with one
// mint signal for the step being taken and monospaced telemetry.
var (
	canvas     = ui.Hex("#080a0d")
	railTone   = ui.Hex("#0c1116")
	panel      = ui.Hex("#10161c")
	panelUp    = ui.Hex("#151d24")
	textMain   = ui.Hex("#e6edf3")
	textSoft   = ui.Hex("#9aa7b4")
	textMuted  = ui.Hex("#83909d")
	hairline   = ui.RGBA(255, 255, 255, 0.07)
	hairlineUp = ui.RGBA(255, 255, 255, 0.11)

	mint    = ui.Hex("#5be0ce")
	mintDim = ui.RGBA(91, 224, 206, 0.10)
	danger  = ui.Hex("#ff6e6e")
	warning = ui.Hex("#f1b45a")
	success = ui.Hex("#57d9a3")
)

var theme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = canvas
	t.Surface = panelUp
	t.SurfaceHover = ui.Hex("#19212a")
	t.SurfacePressed = ui.Hex("#1f2933")
	t.Border = hairlineUp
	t.Text = textMain
	t.TextMuted = textMuted
	t.Accent = mint
	t.AccentHover = ui.Hex("#78ebdc")
	t.AccentPressed = ui.Hex("#43c4b4")
	t.AccentText = ui.Hex("#06120f")
	t.Danger = danger
	t.Warning = warning
	t.Success = success
	t.Selection = ui.RGBA(91, 224, 206, 0.22)
	t.FocusRingWidth = 1.5
	return t
}()

// view draws the control console beside the browser, the page as the target.
func (a *app) view(c *ui.Context) {
	c.SetTheme(theme)
	bar := c.TitleBar()

	ui.Column(c).Fill().Background(canvas).Children(func() {
		ui.Row(c).Grow(1).Children(func() {
			a.railColumn(c, bar)
			a.stage(c, bar)
		})
		a.statusBar(c)
	})

	a.exportDialog(c)
	a.historyDialog(c)
	a.chaptersDialog(c)
	a.stopAlert(c)
	a.deleteAlert(c)
}

func (a *app) railColumn(c *ui.Context, bar ui.TitleBar) {
	ui.Column(c).Width(304).FillHeight().Padding(bar.Bottom+16, 20, 18, 20).Gap(18).
		Background(railTone).BorderWidth(0, 1, 0, 0).BorderColor(hairline).Children(func() {
		a.brand(c)
		a.targetSection(c)
		a.jobSection(c)
		a.actionSection(c)
		a.sessionSection(c)
		a.queueSection(c)

		ui.Spacer(c)

		a.historySection(c)
	})
}

func (a *app) brand(c *ui.Context) {
	ui.Row(c).Gap(6).AlignItems(ui.Baseline).DragWindow().Children(func() {
		ui.Text(c, ">").Font("monospace").FontSize(12).Bold().TextColor(mint)
		ui.Text(c, "justnovel").Font("monospace").FontSize(12).Bold().
			LetterSpacing(0.6).TextColor(textMain)
	})
	ui.Divider(c).Background(hairline)
}

// sectionLabel frames a block of the console.
func sectionLabel(c *ui.Context, index, label string) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, index).Font("monospace").FontSize(9).TextColor(mint)
		ui.Text(c, label).Font("monospace").FontSize(9).LetterSpacing(1.4).TextColor(textMuted)
	})
}

func (a *app) targetSection(c *ui.Context) {
	sectionLabel(c, "01", "TARGET")

	if a.catalog == nil {
		ui.Text(c, "NO TARGET").Font("monospace").FontSize(10).TextColor(textMuted)
		ui.Text(c, "在右侧填入目录页地址").FontSize(10).TextColor(textSoft)
		return
	}

	catalog := *a.catalog
	ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
		ui.Text(c, "#").Font("monospace").FontSize(11).Bold().TextColor(mint)
		ui.Text(c, catalog.Title).FontSize(12).Bold().TextColor(textMain).Grow(1)
	})

	meta := fmt.Sprintf("%d CH", len(catalog.Chapters))
	if catalog.Author != "" && catalog.Author != "Unknown" {
		meta = catalog.Author + " · " + meta
	}
	ui.Text(c, meta).Font("monospace").FontSize(9).TextColor(textSoft)
}

func (a *app) jobSection(c *ui.Context) {
	sectionLabel(c, "02", "JOB")

	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Text(c, phaseToken(a.job.Phase)).Font("monospace").FontSize(9).
			LetterSpacing(0.8).TextColor(phaseColor(a.job.Phase))
		ui.Text(c, a.job.Title()).FontSize(12).Bold().TextColor(textMain)
	})

	if snap := a.job.Snap; snap != nil && (a.job.Phase == PhaseDownloading || a.job.Phase == PhasePaused) {
		ui.Row(c).AlignItems(ui.Baseline).Gap(4).Children(func() {
			ui.Text(c, itoa(snap.Completed)).Font("monospace").FontSize(26).Bold().
				TextColor(textMain).FontFeatures("tnum")
			ui.Text(c, "/"+itoa(snap.Total)).Font("monospace").FontSize(9).
				TextColor(textMuted).FontFeatures("tnum")
			ui.Spacer(c)
			ui.Textf(c, "%d%%", int(snap.Fraction()*100)).Font("monospace").FontSize(9).
				TextColor(mint).FontFeatures("tnum")
		})
		ui.Progress(c, snap.Fraction()).TextColor(mint).Background(ui.Hex("#1a222a"))
		ui.Column(c).Gap(2).Children(func() {
			ui.Row(c).Gap(5).Children(func() {
				ui.Text(c, "NOW").Font("monospace").FontSize(9).TextColor(mint)
				ui.Textf(c, "#%d", snap.CurrentIndex).Font("monospace").FontSize(9).TextColor(mint)
				ui.Text(c, snap.CurrentTitle).FontSize(10).TextColor(textSoft).Grow(1)
			})
			if snap.HasLast {
				ui.Row(c).Gap(5).Children(func() {
					ui.Text(c, "LAST").Font("monospace").FontSize(9).TextColor(textMuted)
					ui.Textf(c, "#%d", snap.LastIndex).Font("monospace").FontSize(9).TextColor(textMuted)
					ui.Text(c, snap.LastTitle).FontSize(10).TextColor(textMuted).Grow(1)
				})
			}
			if snap.Transport == TransportBrowser {
				ui.Text(c, "VIA BROWSER").Font("monospace").FontSize(9).TextColor(warning)
			}
		})
	} else {
		ui.Text(c, a.statusDetail()).FontSize(10).LineHeight(1.5).TextColor(textSoft)
	}

	if a.awaiting {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(4, 4).Radius(2).Background(warning)
			ui.Text(c, "请在浏览器面板完成验证").FontSize(10).TextColor(warning)
		})
	}
}

func phaseToken(phase JobPhase) string {
	switch phase {
	case PhaseSessionRequired:
		return "AUTH"
	case PhaseParsing:
		return "PARSE"
	case PhaseStarting:
		return "START"
	case PhaseDownloading:
		return "RUN"
	case PhasePaused:
		return "HOLD"
	case PhaseBuilding:
		return "BUILD"
	case PhaseFinished:
		return "DONE"
	case PhaseStopped:
		return "STOP"
	case PhaseFailed:
		return "ERR"
	default:
		return "IDLE"
	}
}

func phaseColor(phase JobPhase) ui.Color {
	switch phase {
	case PhaseFinished:
		return success
	case PhaseFailed:
		return danger
	case PhasePaused, PhaseSessionRequired:
		return warning
	case PhaseDownloading, PhaseStarting, PhaseParsing, PhaseBuilding:
		return mint
	default:
		return textMuted
	}
}

// actionSection is the three steps of the job, the next one filled.
// currentStep names the action the user should take next.
func (a *app) currentStep() string {
	if a.catalog != nil && !a.job.Busy() {
		return "download"
	}
	if a.canPause() || a.canResume() {
		return "download"
	}
	if a.job.Phase == PhaseSessionRequired || a.session.State().Cookies > 0 {
		return "sync"
	}
	return "open"
}

func downloadActionable(a *app) bool {
	return a.canPause() || a.canResume() || (a.catalog != nil && !a.job.Busy())
}

func (a *app) downloadButtonTitle() string {
	switch {
	case a.canPause():
		return "暂停下载"
	case a.canResume():
		return "继续下载"
	default:
		return "开始下载"
	}
}

func firstRune(title string) string {
	if runes := []rune(trimParagraph(title)); len(runes) > 0 {
		return string(runes[0])
	}
	return "书"
}

func (a *app) actionSection(c *ui.Context) {
	step := a.currentStep()

	ui.Column(c).Gap(5).Children(func() {
		if step == "open" && a.canParse() {
			ui.PrimaryButton(c, "打开目录").FontSize(10).FillWidth().OnClick(a.openSource)
		} else {
			ui.Button(c, "打开目录").FontSize(10).FillWidth().Disabled(!a.canParse()).OnClick(a.openSource)
		}

		if step == "sync" && a.canParse() {
			ui.PrimaryButton(c, "同步会话").FontSize(10).FillWidth().OnClick(func() { go a.syncSession() })
		} else {
			ui.Button(c, "同步会话").FontSize(10).FillWidth().Disabled(!a.canParse()).OnClick(func() { go a.syncSession() })
		}

		// Paused, failed and finished-with-chapters jobs are resumed or
		// exported from the rail under the page, where their state shows.
		hideDownload := (a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed) ||
			(a.job.Phase == PhaseFinished && len(a.chapters) > 0)
		if !hideDownload {
			label := a.downloadButtonTitle()
			if step == "download" && downloadActionable(a) {
				ui.PrimaryButton(c, label).FontSize(10).FillWidth().OnClick(a.toggleDownload)
			} else {
				ui.Button(c, label).FontSize(10).FillWidth().Disabled(!downloadActionable(a)).OnClick(a.toggleDownload)
			}
		}
	})

}

func textAction(c *ui.Context, label string, color ui.Color, action func()) {
	ui.Text(c, label).FontSize(10).TextColor(color).Padding(4, 0).OnClick(action)
}

func (a *app) sessionSection(c *ui.Context) {
	sectionLabel(c, "03", "SESSION")

	state := a.session.State()
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(4, 4).Radius(2).Background(sessionDot(state))
		ui.Text(c, a.source.Name).FontSize(10).TextColor(textSoft)
	})

	switch {
	case state.Manual:
		ui.Text(c, "等待人工验证").FontSize(10).TextColor(warning)
	case state.Loading:
		ui.Text(c, "正在加载站点页面").FontSize(10).TextColor(textMuted)
	case state.Cookies > 0:
		ui.Text(c, "已同步").FontSize(10).TextColor(textMuted)
	default:
		ui.Text(c, "尚未同步").FontSize(10).TextColor(textMuted)
	}
	if state.LastError != "" {
		ui.Text(c, state.LastError).FontSize(9).TextColor(danger)
	}
	if a.lastSaved != "" {
		ui.Text(c, "已导出 "+filepath.Base(a.lastSaved)).FontSize(9).TextColor(textSoft)
	}
}

// queueSection shows the chapters the job will take next.
func (a *app) queueSection(c *ui.Context) {
	sectionLabel(c, "04", "QUEUE")

	if a.catalog == nil || a.next >= len(a.catalog.Chapters) {
		ui.Text(c, "QUEUE EMPTY").Font("monospace").FontSize(9).TextColor(textMuted)
		return
	}

	for i := 1; i <= 2 && a.next+i < len(a.catalog.Chapters); i++ {
		chapter := a.catalog.Chapters[a.next+i]
		ui.Row(c).Gap(5).Children(func() {
			ui.Text(c, "NEXT").Font("monospace").FontSize(9).TextColor(textMuted).Width(26)
			ui.Textf(c, "#%d", chapter.Order).Font("monospace").FontSize(9).TextColor(textMuted)
			ui.Text(c, chapter.Title).FontSize(10).TextColor(textSoft).Grow(1)
		})
	}
}

// historySection is the last numbered block: the saved snapshots.
func (a *app) historySection(c *ui.Context) {
	sectionLabel(c, "05", "HISTORY")

	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "%d RECORDS", a.history.Count()).Font("monospace").FontSize(9).TextColor(textMuted)
		ui.Spacer(c)
		textAction(c, "打开下载历史", textSoft, a.openHistory)
	})
}

// statusBar closes the window like an instrument: source, host, session and
// cache along the bottom edge.
func (a *app) statusBar(c *ui.Context) {
	state := a.session.State()
	ui.Row(c).Height(24).Padding(0, 12).Gap(14).Background(ui.Hex("#07090b")).
		BorderWidth(1, 0, 0, 0).BorderColor(hairline).AlignItems(ui.Center).Children(func() {
		chip := func(label, value string, color ui.Color) {
			ui.Text(c, label).Font("monospace").FontSize(8).TextColor(textMuted)
			ui.Text(c, value).Font("monospace").FontSize(9).TextColor(color)
		}
		host := mustParseURL(a.urlText).Host
		if host == "" {
			host = "—"
		}
		chip("SRC", a.source.Name, textSoft)
		chip("HOST", host, textSoft)
		chip("COOKIES", itoa(state.Cookies), textSoft)
		if a.history != nil {
			chip("CACHE", humanBytes(a.history.ByteCount()), textSoft)
		}
		ui.Spacer(c)
		ui.Text(c, "仅抓取你有权访问的公开页面").FontSize(10).TextColor(textMuted)
	})
}

func sessionDot(state sessionView) ui.Color {
	switch {
	case state.Loading:
		return mint
	case state.Manual:
		return warning
	case state.Cookies > 0:
		return success
	default:
		return hairlineUp
	}
}

func (a *app) stage(c *ui.Context, bar ui.TitleBar) {
	ui.Column(c).Grow(1).FillHeight().Padding(bar.Bottom+16, 18, 14, 18).Gap(8).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, "BROWSER").Font("monospace").FontSize(9).LetterSpacing(1.4).TextColor(mint)
			state := a.session.State()
			ui.Box(c).Size(4, 4).Radius(2).Background(sessionDot(state))
			ui.Text(c, state.Status).FontSize(9).TextColor(textMuted).Grow(1)

			if a.showBrowser {
				textAction(c, "隐藏", textMuted, func() { a.showBrowser = false })
			} else {
				textAction(c, "显示", mint, func() { a.showBrowser = true })
			}
		})

		a.sourceBar(c)

		state := a.session.State()
		switch {
		case !a.showBrowser:
			ui.Column(c).Grow(1).Radius(4).Background(panel).Border(1, hairline).
				AlignItems(ui.Center).Justify(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "BROWSER HIDDEN").Font("monospace").FontSize(9).TextColor(textMuted)
				ui.Text(c, "验证站点、查看目录时再打开它").FontSize(10).TextColor(textSoft)
				ui.Button(c, "显示浏览器").FontSize(10).OnClick(func() { a.showBrowser = true })
			})
		case a.session.view != nil && state.Loaded:
			ui.WebView(c, a.session.view).Grow(1).Radius(4).
				Border(1, hairlineUp).
				InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.04))
		default:
			ui.Column(c).Grow(1).Radius(4).Background(panel).Border(1, hairline).
				AlignItems(ui.Center).Justify(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "> 等待目标").Font("monospace").FontSize(12).TextColor(textMuted)
				ui.Text(c, "在地址栏填入目录页地址，点「打开目录」").FontSize(10).TextColor(textSoft)
			})
		}

		if a.showRail() {
			a.rail(c)
		}
	})
}

func (a *app) sourceBar(c *ui.Context) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "GET").Font("monospace").FontSize(9).Bold().TextColor(mint).
			Padding(4, 7).Radius(3).Background(mintDim)
		ui.TextInput(c, &a.urlText).Placeholder(a.source.Placeholder).
			Font("monospace").FontSize(10).Grow(1)

		names := make([]string, len(Sources))
		for i, source := range Sources {
			names[i] = source.Name
		}
		chosen := a.source.Name
		ui.Select(c, &chosen, names).FontSize(10).Width(116).OnChange(func() {
			for _, source := range Sources {
				if source.Name == chosen {
					a.selectSource(source.Kind)
				}
			}
		})
	})

	if strings.TrimSpace(a.urlText) != "" && a.source.CatalogURL(a.urlText) == nil {
		ui.Text(c, a.source.InvalidText).FontSize(9).TextColor(danger)
	}
}

func (a *app) showRail() bool {
	switch a.job.Phase {
	case PhasePaused, PhaseFinished, PhaseFailed:
		return true
	}
	return len(a.chapters) > 0
}

func (a *app) rail(c *ui.Context) {
	ui.Divider(c).Background(hairline)

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, ">").Font("monospace").FontSize(11).Bold().TextColor(railColor(a.job.Phase))
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, a.railTitle()).FontSize(10).Bold().TextColor(railColor(a.job.Phase))
			if message := a.railMessage(); message != "" {
				ui.Row(c).Children(func() {
					ui.Text(c, message).FontSize(10).TextColor(textMuted)
				})
			}
		})

		if a.canResume() {
			ui.PrimaryButton(c, "继续下载").FontSize(10).OnClick(a.resumeDownload)
		}
		if a.job.Phase == PhaseFailed && !a.running && a.catalog != nil && a.next < len(a.catalog.Chapters) {
			if a.canResume() {
				ui.Button(c, "跳过当前章").FontSize(10).OnClick(a.skipChapter)
			} else {
				ui.PrimaryButton(c, "跳过当前章").FontSize(10).OnClick(a.skipChapter)
			}
		}
		if a.canExportSession() {
			ui.Button(c, "导出当前内容").FontSize(10).OnClick(a.openExportDialog)
		}
		if len(a.chapters) > 0 {
			ui.Button(c, "查看已下载章节").FontSize(10).OnClick(func() { a.showChapters = true })
		}
		if a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed {
			ui.Box(c).Width(1).Height(16).Background(hairline).Margin(4, 0, 4, 0)
			ui.Button(c, "停止并清空").FontSize(10).OnClick(func() { a.pendingStop = true })
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
		return "下载中断"
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
		return "当前还没有成功章节，可以继续、跳过当前章或重新开始整次下载。"
	case PhasePaused:
		if snap := a.job.Snap; snap != nil {
			return fmt.Sprintf("当前会话已保留 %d / %d 章。", snap.Completed, snap.Total)
		}
		return "当前会话已保留。"
	case PhaseFinished:
		return a.job.Message
	default:
		if len(a.chapters) > 0 {
			return "可以检查已下载章节，并决定哪些内容进入导出结果。"
		}
		return ""
	}
}

func (a *app) exportDialog(c *ui.Context) {
	ui.DialogBase(c, &a.showExport, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(3, 5, 7, 0.64))
		sheet.Padding(20).Radius(6).Background(panel).Border(1, hairlineUp).
			Shadow(0, 18, 44, 0, ui.RGBA(0, 0, 0, 0.5))

		ui.Column(c).Width(600).Gap(16).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				sectionLabel(c, "", "EXPORT")
				ui.Text(c, "导出设置").FontSize(13).Bold().TextColor(textMain)
				ui.Text(c, "只改变导出的排版与附加信息，不影响抓取").FontSize(10).TextColor(textMuted)
			})

			ui.Row(c).Gap(22).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Gap(14).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						sectionLabel(c, "", "AUTHOR")
						ui.TextInput(c, &a.draft.Author).Placeholder("作者").FontSize(10)
					})

					ui.Column(c).Gap(4).Children(func() {
						sectionLabel(c, "", "FONT")
						fonts := make([]string, len(FontPresets))
						for i, preset := range FontPresets {
							fonts[i] = preset.DisplayName()
						}
						if a.exportFont == "" {
							a.exportFont = a.draft.BodyFont.DisplayName()
						}
						ui.Select(c, &a.exportFont, fonts).FontSize(10)
					})

					ui.Column(c).Gap(4).Children(func() {
						sectionLabel(c, "", "FRONT PAGE")
						ui.TextInput(c, &a.draft.FrontText).Placeholder(defaultFrontPageBodyText).
							Multiline(true).MinHeight(60).FontSize(10)
						ui.Checkbox(c, &a.draft.ShowSource, "首页显示来源链接").FontSize(9)
					})
				})

				ui.Column(c).Width(150).Gap(6).Children(func() {
					sectionLabel(c, "", "COVER")
					if a.coverPreview != nil {
						ui.Image(c, a.coverPreview).Size(150, 200).Radius(3).Border(1, hairlineUp)
					} else {
						ui.Column(c).Size(150, 200).Radius(3).Background(panelUp).Border(1, hairline).
							AlignItems(ui.Center).Justify(ui.Center).Gap(5).Children(func() {
							ui.Text(c, firstRune(a.draft.Title)).FontSize(30).Bold().TextColor(mint)
							ui.Text(c, "NO COVER").Font("monospace").FontSize(8).TextColor(textMuted)
						})
					}
					label := "选择封面"
					if len(a.draft.CoverImage) > 0 {
						label = "更换封面"
					}
					ui.Button(c, label).FontSize(10).OnClick(a.importCover)
					if len(a.draft.CoverImage) > 0 {
						ui.Button(c, "移除封面").FontSize(10).OnClick(a.removeCover)
					}
				})
			})

			ui.Divider(c).Background(hairline)

			ui.Row(c).Gap(6).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").FontSize(10).OnClick(func() { a.showExport = false })
				ui.PrimaryButton(c, "导出 EPUB").FontSize(10).OnClick(a.confirmExport)
			})
		})
	})
}

func (a *app) historyDialog(c *ui.Context) {
	height := float32(160 + min(len(a.historyList), 5)*34)
	ui.DialogBase(c, &a.showHistory, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(3, 5, 7, 0.64))
		sheet.Padding(20).Radius(6).Background(panel).Border(1, hairlineUp).
			Shadow(0, 18, 44, 0, ui.RGBA(0, 0, 0, 0.5))

		ui.Column(c).Width(720).Height(height).Gap(14).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				sectionLabel(c, "", "HISTORY")
				ui.Text(c, "下载历史").FontSize(13).Bold().TextColor(textMain)
				ui.Text(c, "这里保存可再次导出的 EPUB 快照").FontSize(10).TextColor(textMuted)
			})

			ui.Row(c).Grow(1).Gap(18).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(220).Gap(1).Children(func() {
					if len(a.historyList) == 0 {
						ui.Text(c, "还没有记录").FontSize(10).TextColor(textSoft)
						ui.Text(c, "开始一次下载后，这里会保存可再次导出的 EPUB 快照。").FontSize(9).TextColor(textMuted)
						return
					}
					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).Gap(1).Children(func() {
							for _, record := range a.historyList {
								selected := record.ID == a.selectedID
								ui.Row(c).Height(40).Gap(7).AlignItems(ui.Center).Children(func() {
									bar := ui.Box(c).Width(2).Height(20).Radius(1).Background(ui.Transparent)
									if selected {
										bar.Background(mint)
									}
									ui.Column(c).Gap(1).Grow(1).Children(func() {
										color := textSoft
										if selected {
											color = textMain
										}
										ui.Text(c, record.Title).FontSize(10).TextColor(color)
										ui.Textf(c, "%s · %s", record.ProgressLabel(), record.UpdatedAt.Format("01-02")).
											Font("monospace").FontSize(8).TextColor(textMuted)
									})
								}).OnClick(func() { a.selectHistory(record.ID) })
							}
						})
					})
				})

				ui.Divider(c).Background(hairline)

				ui.Column(c).Grow(1).Gap(10).Children(func() {
					record, ok := a.selectedRecord()
					if !ok {
						ui.Spacer(c)
						ui.Text(c, "还没有下载历史").FontSize(11).TextColor(textSoft).TextAlign(ui.Center)
						ui.Spacer(c)
						return
					}

					ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
						if a.historyCover != nil {
							ui.Image(c, a.historyCover).Size(84, 112).Radius(3).Border(1, hairlineUp)
						}
						ui.Column(c).Gap(2).Grow(1).Children(func() {
							ui.Text(c, record.Title).FontSize(13).Bold().TextColor(textMain)
							if record.Author != "" && record.Author != "Unknown" {
								ui.Text(c, record.Author).FontSize(9).TextColor(textSoft)
							}
							ui.Textf(c, "%s · %s", completeLabel(record), record.SourceHost()).
								Font("monospace").FontSize(8).TextColor(mint)
						})
					})

					ui.Column(c).Gap(4).Children(func() {
						detailRow(c, "PROGRESS", record.ProgressLabel())
						detailRow(c, "UPDATED", record.UpdatedAt.Format("2006-01-02 15:04"))
						detailRow(c, "CREATED", record.CreatedAt.Format("2006-01-02 15:04"))
					})

					ui.Text(c, historyMessage(record)).FontSize(10).LineHeight(1.5).TextColor(textSoft)
					if a.historyError != "" {
						ui.Text(c, a.historyError).FontSize(10).TextColor(danger)
					}
				})
			})

			ui.Divider(c).Background(hairline)

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "关闭").FontSize(10).OnClick(func() { a.showHistory = false })
				ui.Button(c, "删除记录").FontSize(10).TextColor(danger).Disabled(a.selectedID == "").
					OnClick(func() { a.pendingDelete = true })
				ui.Spacer(c)
				_, ok := a.selectedRecord()
				ui.PrimaryButton(c, "导出 EPUB").FontSize(10).Disabled(!ok).OnClick(a.exportHistory)
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
		ui.Text(c, title).Font("monospace").FontSize(9).TextColor(textMuted).Width(72)
		ui.Text(c, value).Font("monospace").FontSize(9).TextColor(textMain).Grow(1)
	})
}

func (a *app) chaptersDialog(c *ui.Context) {
	height := float32(150 + min(len(a.chapters), 6)*30)
	ui.DialogBase(c, &a.showChapters, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(3, 5, 7, 0.64))
		sheet.Padding(20).Radius(6).Background(panel).Border(1, hairlineUp).
			Shadow(0, 18, 44, 0, ui.RGBA(0, 0, 0, 0.5))

		ui.Column(c).Width(520).Height(height).Gap(12).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				sectionLabel(c, "", "CHAPTERS")
				ui.Text(c, "已下载章节").FontSize(13).Bold().TextColor(textMain)
				ui.Text(c, "删除后会从导出结果中排除，但不会回退下载进度").FontSize(10).TextColor(textMuted)
			})

			ui.Textf(c, "%d CHAPTERS", len(a.chapters)).Font("monospace").FontSize(8).TextColor(textMuted)

			ui.List(c, nil, len(a.chapters), func(i int) {
				chapter := a.chapters[i]
				ui.Row(c).Padding(5, 0).Height(30).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Textf(c, "%03d", chapter.Order).Font("monospace").FontSize(8).
						TextColor(textMuted).Width(30)
					ui.Text(c, chapter.Title).FontSize(10).TextColor(textMain).Grow(1)
					if !a.job.Busy() {
						ui.Text(c, "移除").FontSize(10).TextColor(textSoft).
							Padding(4, 6).OnClick(func() { a.removeChapter(chapter.Order) })
					}
				})
				ui.Divider(c).Background(hairline)
			}).Grow(1)

			ui.Row(c).Gap(6).Justify(ui.End).Children(func() {
				ui.Button(c, "关闭").FontSize(10).OnClick(func() { a.showChapters = false })
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
