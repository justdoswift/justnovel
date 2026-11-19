package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// The app's visual system: an ink canvas, paper-like surfaces and a single
// persimmon accent that marks the step the user takes next.
var (
	ink         = ui.Hex("#0b0b0d")
	panel       = ui.Hex("#111114")
	raised      = ui.Hex("#17171b")
	hairline    = ui.RGBA(255, 255, 255, 0.07)
	hairlineLit = ui.RGBA(255, 255, 255, 0.13)

	textPrimary   = ui.Hex("#f4f2ee")
	textSecondary = ui.Hex("#b5b1aa")
	textMuted     = ui.Hex("#7c7973")

	accent     = ui.Hex("#e07856")
	accentSoft = ui.Hex("#f2a98d")
	success    = ui.Hex("#6fcf9b")
	warning    = ui.Hex("#e9b45f")
	danger     = ui.Hex("#e56f63")
)

// theme keeps the native controls in the same ink as the window.
var theme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ink
	t.Surface = ui.RGBA(255, 255, 255, 0.055)
	t.SurfaceHover = ui.RGBA(255, 255, 255, 0.09)
	t.SurfacePressed = ui.RGBA(255, 255, 255, 0.13)
	t.Border = hairlineLit
	t.Text = textPrimary
	t.TextMuted = textMuted
	t.Accent = accent
	t.AccentHover = ui.Hex("#e88a6b")
	t.AccentPressed = ui.Hex("#c96546")
	t.AccentText = ui.Hex("#1a0f0b")
	t.Danger = danger
	t.Warning = warning
	t.Success = success
	t.Selection = ui.RGBA(224, 120, 86, 0.35)
	return t
}()

// view builds the window: a drawn title bar, an instrument-like sidebar and
// the browser panel that is the app's workbench.
func (a *app) view(c *ui.Context) {
	c.SetTheme(theme)
	bar := c.TitleBar()

	ui.Column(c).Fill().Background(ink).Children(func() {
		a.topBar(c, bar)

		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			a.stage(c)
		})
	})

	a.exportDialog(c)
	a.historyDialog(c)
	a.chaptersDialog(c)
	a.stopAlert(c)
	a.deleteAlert(c)
}

func (a *app) topBar(c *ui.Context, bar ui.TitleBar) {
	height := float32(52)
	if bar.Bottom > 0 {
		height = bar.Bottom + 12
	}

	ui.Row(c).Height(height).Padding(0, 22, 0, bar.Left+20).Gap(8).
		AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, "JustNovel").Font("Georgia").FontSize(17).TextColor(textPrimary).LetterSpacing(0.2)
		ui.Text(c, "网页小说 → EPUB").FontSize(10).TextColor(textMuted).LetterSpacing(0.6).Margin(4, 0, 0, 0)

		ui.Spacer(c)

		step := a.currentStep()
		if step == "open" && a.canParse() {
			ui.PrimaryButton(c, "打开目录").OnClick(a.openSource)
		} else {
			ui.Button(c, "打开目录").Disabled(!a.canParse()).OnClick(a.openSource)
		}
		if step == "sync" && a.canParse() {
			ui.PrimaryButton(c, "同步会话").OnClick(func() { go a.syncSession() })
		} else {
			ui.Button(c, "同步会话").Disabled(!a.canParse()).OnClick(func() { go a.syncSession() })
		}

		ui.Box(c).Width(1).Height(20).Background(hairline).Margin(4, 0, 4, 0)

		if step == "download" && downloadActionable(a) && a.job.Phase != PhasePaused && a.job.Phase != PhaseFailed {
			ui.PrimaryButton(c, a.downloadButtonTitle()).OnClick(a.toggleDownload)
		} else {
			ui.Button(c, a.downloadButtonTitle()).Disabled(!downloadActionable(a)).OnClick(a.toggleDownload)
		}

		ui.Box(c).Width(1).Height(20).Background(hairline).Margin(4, 0, 4, 0)

		ui.Button(c, "导出 EPUB").Disabled(!a.canExportSession()).OnClick(a.openExportDialog)
		ui.Button(c, "下载历史").OnClick(a.openHistory)
	})
	ui.Divider(c).Background(hairline)
}

// currentStep names the action the user should take next: open the catalog,
// sync the session, or download.
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

// downloadActionable reports whether the download button does something now.
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

func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Width(300).Padding(26, 24, 26, 24).Gap(26).
		BorderWidth(0, 1, 0, 0).BorderColor(hairline).Children(func() {
		a.statusSection(c)

		if a.catalog != nil {
			a.catalogSection(c, *a.catalog)
		}

		if a.catalog == nil && !a.job.Busy() {
			a.firstRunSteps(c)
		}

		a.sessionSection(c)

		ui.Spacer(c)

		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "仅用于抓取你有权访问的公开页面内容").FontSize(11).TextColor(textMuted)
			if a.history != nil {
				ui.Textf(c, "本地缓存 %s", humanBytes(a.history.ByteCount())).FontSize(11).TextColor(textMuted)
			}
		})
	})
}

// sectionLabel is the small caps-like label above each block.
func sectionLabel(c *ui.Context, label string) {
	ui.Text(c, label).FontSize(10).LetterSpacing(0.4).TextColor(textMuted)
}

func (a *app) statusSection(c *ui.Context) {
	sectionLabel(c, "状态")

	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(2).Height(22).Radius(1).Background(phaseColor(a.job.Phase))
		ui.Text(c, a.job.Title()).FontSize(19).Bold().TextColor(textPrimary)
	})

	if snap := a.job.Snap; snap != nil && (a.job.Phase == PhaseDownloading || a.job.Phase == PhasePaused) {
		ui.Row(c).AlignItems(ui.Baseline).Gap(6).Children(func() {
			ui.Text(c, itoa(snap.Completed)).FontSize(28).Bold().TextColor(textPrimary).FontFeatures("tnum")
			ui.Text(c, "/ "+itoa(snap.Total)).FontSize(12).TextColor(textMuted).FontFeatures("tnum")
			ui.Spacer(c)
			ui.Textf(c, "%d%%", int(snap.Fraction()*100)).FontSize(11).TextColor(textSecondary).FontFeatures("tnum")
		})
		ui.Progress(c, snap.Fraction()).TextColor(accent)
		ui.Column(c).Gap(4).Children(func() {
			verb := "正在下载"
			if a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed {
				verb = "待下载"
			}
			ui.Textf(c, "%s 第 %d 章 %s", verb, snap.CurrentIndex, snap.CurrentTitle).FontSize(12).TextColor(textSecondary)
			if snap.HasLast {
				ui.Textf(c, "最近完成 第 %d 章 %s", snap.LastIndex, snap.LastTitle).FontSize(11).TextColor(textMuted)
			}
			if snap.Transport == TransportBrowser {
				ui.Text(c, "浏览器回退抓取").FontSize(11).TextColor(textMuted)
			}
		})
	} else {
		ui.Text(c, a.statusDetail()).FontSize(12).LineHeight(1.5).TextColor(textSecondary)
	}

	if a.awaiting {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(5, 5).Radius(3).Background(warning)
			ui.Text(c, "请在浏览器面板完成验证").FontSize(11).TextColor(warning)
		})
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
		return accent
	default:
		return hairlineLit
	}
}

// firstRune is the character a book is marked with.
func firstRune(title string) string {
	if runes := []rune(trimParagraph(title)); len(runes) > 0 {
		return string(runes[0])
	}
	return "书"
}

// bookPlate is the book's mark: its first character set as a small plate.
func bookPlate(c *ui.Context, title string) {
	ui.Column(c).Size(44, 60).Radius(6).Background(ui.RGBA(224, 120, 86, 0.10)).
		Border(1, ui.RGBA(224, 120, 86, 0.22)).AlignItems(ui.Center).Justify(ui.Center).
		Children(func() {
			ui.Text(c, firstRune(title)).Font("Songti SC").FontSize(22).TextColor(accentSoft)
		})
}

func (a *app) catalogSection(c *ui.Context, catalog BookCatalog) {
	sectionLabel(c, "目录")

	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		bookPlate(c, catalog.Title)
		ui.Column(c).Gap(4).Grow(1).Children(func() {
			ui.Text(c, catalog.Title).Font("Songti SC").FontSize(19).TextColor(textPrimary)
			meta := fmt.Sprintf("%d 章", len(catalog.Chapters))
			if catalog.Author != "" && catalog.Author != "Unknown" {
				meta = catalog.Author + " · " + meta
			}
			ui.Text(c, meta).FontSize(11).TextColor(textSecondary)
		})
	})

	if a.canExportSession() {
		ui.Row(c).Children(func() {
			ui.Text(c, "导出当前内容").FontSize(11).TextColor(accentSoft).OnClick(a.openExportDialog)
		})
	}
}

// firstRunSteps explains the three steps while the sidebar has no book yet.
func (a *app) firstRunSteps(c *ui.Context) {
	sectionLabel(c, "接下来")
	ui.Column(c).Gap(6).Children(func() {
		for _, step := range []string{
			"1  打开目录页，在浏览器确认内容",
			"2  完成站点验证，同步会话",
			"3  解析目录后开始下载",
		} {
			ui.Text(c, step).FontSize(11).TextColor(textMuted)
		}
	})
}

func (a *app) sessionSection(c *ui.Context) {
	sectionLabel(c, "会话")

	state := a.session.State()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(5, 5).Radius(3).Background(sessionDot(state))
		ui.Text(c, a.source.SessionTitle).FontSize(11).TextColor(textSecondary)
	})

	switch {
	case state.Manual:
		ui.Text(c, "等待人工验证").FontSize(11).TextColor(warning)
	case state.Loading:
		ui.Text(c, "正在加载站点页面").FontSize(11).TextColor(textMuted)
	case state.Cookies > 0:
		ui.Textf(c, "已同步 %d 个 Cookie", state.Cookies).FontSize(11).TextColor(textMuted)
	default:
		ui.Text(c, "尚未同步").FontSize(11).TextColor(textMuted)
	}
	if state.LastError != "" {
		ui.Text(c, state.LastError).FontSize(11).TextColor(danger)
	}
	if a.lastSaved != "" {
		ui.Text(c, "已导出 "+filepath.Base(a.lastSaved)).FontSize(11).TextColor(textSecondary)
	}
}

func sessionDot(state sessionView) ui.Color {
	switch {
	case state.Loading:
		return accent
	case state.Manual:
		return warning
	case state.Cookies > 0:
		return success
	default:
		return hairlineLit
	}
}

func (a *app) stage(c *ui.Context) {
	ui.Column(c).Grow(1).Padding(24, 24, 24, 24).Gap(14).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "浏览器会话").FontSize(13).Bold().TextColor(textPrimary)
			state := a.session.State()
			ui.Box(c).Size(5, 5).Radius(3).Background(sessionDot(state))
			ui.Text(c, state.Status).FontSize(11).TextColor(textMuted)

			ui.Spacer(c)

			label := "显示浏览器"
			if a.showBrowser {
				label = "隐藏浏览器"
			}
			ui.Button(c, label).OnClick(func() { a.showBrowser = !a.showBrowser })
		})

		a.sourceBar(c)

		if a.showBrowser && a.session.view != nil {
			ui.WebView(c, a.session.view).Grow(1).Radius(12).Border(1, hairlineLit)
		} else {
			ui.Column(c).Grow(1).Radius(12).Background(panel).Border(1, hairline).
				AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "浏览器面板已隐藏").Font("Songti SC").FontSize(17).TextColor(textSecondary)
				ui.Text(c, "验证站点、查看目录时再打开它").FontSize(11).TextColor(textMuted)
				ui.Button(c, "显示浏览器").OnClick(func() { a.showBrowser = true })
			})
		}

		if a.showRail() {
			a.rail(c)
		}
	})
}

func (a *app) sourceBar(c *ui.Context) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &a.urlText).Placeholder(a.source.Placeholder).Grow(1)

		names := make([]string, len(Sources))
		for i, source := range Sources {
			names[i] = source.Name
		}
		chosen := a.source.Name
		ui.Select(c, &chosen, names).Width(128).OnChange(func() {
			for _, source := range Sources {
				if source.Name == chosen {
					a.selectSource(source.Kind)
				}
			}
		})
	})

	if a.source.CatalogURL(a.urlText) == nil {
		ui.Text(c, a.source.InvalidText).FontSize(10).TextColor(danger)
	}
}

func (a *app) showRail() bool {
	switch a.job.Phase {
	case PhasePaused, PhaseFinished, PhaseFailed:
		return true
	}
	return len(a.chapters) > 0
}

// rail is the strip under the page: what happened, and the ways out of it.
func (a *app) rail(c *ui.Context) {
	ui.Divider(c).Background(hairline)

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(3).Grow(1).Children(func() {
			ui.Text(c, a.railTitle()).FontSize(11).Bold().TextColor(railColor(a.job.Phase))
			if message := a.railMessage(); message != "" {
				ui.Text(c, message).FontSize(11).TextColor(textMuted)
			}
		})

		if a.canResume() {
			ui.PrimaryButton(c, "继续下载").OnClick(a.resumeDownload)
		}
		if a.job.Phase == PhaseFailed && !a.running && a.catalog != nil && a.next < len(a.catalog.Chapters) {
			if a.canResume() {
				ui.Button(c, "跳过当前章").OnClick(a.skipChapter)
			} else {
				ui.PrimaryButton(c, "跳过当前章").OnClick(a.skipChapter)
			}
		}
		if a.canExportSession() {
			ui.Button(c, "导出当前内容").OnClick(a.openExportDialog)
		}
		if len(a.chapters) > 0 {
			ui.Button(c, "查看已下载章节").OnClick(func() { a.showChapters = true })
		}
		if a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed {
			ui.Box(c).Width(1).Height(20).Background(hairline).Margin(4, 0, 4, 0)
			ui.Button(c, "停止并清空").OnClick(func() { a.pendingStop = true })
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
		return textSecondary
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
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		sheet.Padding(24).Radius(16).Background(raised).Border(1, hairlineLit).
			Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.45))
		ui.Column(c).Width(640).Gap(20).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				ui.Text(c, "导出设置").FontSize(19).Bold().TextColor(textPrimary)
				ui.Text(c, "只改变导出的排版与附加信息，不影响抓取").FontSize(11).TextColor(textMuted)
			})

			ui.Row(c).Gap(26).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Gap(18).Children(func() {
					ui.Column(c).Gap(6).Children(func() {
						sectionLabel(c, "作者信息")
						ui.TextInput(c, &a.draft.Author).Placeholder("作者")
					})

					ui.Column(c).Gap(6).Children(func() {
						sectionLabel(c, "正文字体")
						fonts := make([]string, len(FontPresets))
						for i, preset := range FontPresets {
							fonts[i] = preset.DisplayName()
						}
						if a.exportFont == "" {
							a.exportFont = a.draft.BodyFont.DisplayName()
						}
						ui.Select(c, &a.exportFont, fonts)
					})

					ui.Column(c).Gap(6).Children(func() {
						sectionLabel(c, "首页内容")
						ui.TextInput(c, &a.draft.FrontText).Placeholder(defaultFrontPageBodyText).Multiline(true).MinHeight(96)
						ui.Checkbox(c, &a.draft.ShowSource, "首页显示来源链接")
					})
				})

				ui.Column(c).Width(180).Gap(10).Children(func() {
					sectionLabel(c, "封面")
					if a.coverPreview != nil {
						ui.Image(c, a.coverPreview).Size(180, 240).Radius(10).Border(1, hairlineLit)
					} else {
						ui.Column(c).Size(180, 240).Radius(10).Background(ui.RGBA(224, 120, 86, 0.08)).
							Border(1, ui.RGBA(224, 120, 86, 0.20)).AlignItems(ui.Center).Justify(ui.Center).
							Gap(8).Children(func() {
							ui.Text(c, firstRune(a.draft.Title)).Font("Songti SC").FontSize(44).TextColor(accentSoft)
							ui.Text(c, "未设置封面").FontSize(11).TextColor(textMuted)
						})
					}
					label := "选择封面"
					if len(a.draft.CoverImage) > 0 {
						label = "更换封面"
					}
					ui.Button(c, label).OnClick(a.importCover)
					if len(a.draft.CoverImage) > 0 {
						ui.Button(c, "移除封面").OnClick(a.removeCover)
					}
				})
			})

			ui.Divider(c).Background(hairline)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.showExport = false })
				ui.PrimaryButton(c, "导出文件").OnClick(a.confirmExport)
			})
		})
	})
}

func (a *app) historyDialog(c *ui.Context) {
	height := float32(220 + min(len(a.historyList), 5)*40)
	ui.DialogBase(c, &a.showHistory, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		sheet.Padding(24).Radius(16).Background(raised).Border(1, hairlineLit).
			Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.45))
		ui.Column(c).Width(780).Height(height).Gap(18).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				ui.Text(c, "下载历史").FontSize(19).Bold().TextColor(textPrimary)
				ui.Text(c, "这里保存可再次导出的 EPUB 快照").FontSize(11).TextColor(textMuted)
			})

			ui.Row(c).Grow(1).Gap(22).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(250).Gap(2).Children(func() {
					if len(a.historyList) == 0 {
						ui.Text(c, "还没有记录").FontSize(12).TextColor(textSecondary)
						ui.Text(c, "开始一次下载后，这里会保存可再次导出的 EPUB 快照。").FontSize(11).TextColor(textMuted)
						return
					}
					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).Gap(2).Children(func() {
							for _, record := range a.historyList {
								selected := record.ID == a.selectedID
								ui.Row(c).Height(48).Gap(9).AlignItems(ui.Center).Children(func() {
									bar := ui.Box(c).Width(2).Height(26).Radius(1).Background(ui.Transparent)
									if selected {
										bar.Background(accent)
									}
									ui.Column(c).Gap(2).Grow(1).Children(func() {
										color := textSecondary
										if selected {
											color = textPrimary
										}
										ui.Text(c, record.Title).FontSize(12).TextColor(color)
										ui.Textf(c, "%s · %s", record.ProgressLabel(), record.UpdatedAt.Format("01-02")).FontSize(11).TextColor(textMuted)
									})
								}).OnClick(func() { a.selectHistory(record.ID) })
							}
						})
					})
				})

				ui.Divider(c).Background(hairline)

				ui.Column(c).Grow(1).Gap(14).Children(func() {
					record, ok := a.selectedRecord()
					if !ok {
						ui.Spacer(c)
						ui.Text(c, "还没有下载历史").FontSize(15).TextColor(textSecondary).TextAlign(ui.Center)
						ui.Spacer(c)
						return
					}

					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						if a.historyCover != nil {
							ui.Image(c, a.historyCover).Size(104, 140).Radius(8).Border(1, hairlineLit)
						}
						ui.Column(c).Gap(5).Grow(1).Children(func() {
							ui.Text(c, record.Title).Font("Songti SC").FontSize(19).TextColor(textPrimary)
							if record.Author != "" && record.Author != "Unknown" {
								ui.Text(c, record.Author).FontSize(11).TextColor(textSecondary)
							}
							ui.Textf(c, "%s · %s", completeLabel(record), record.SourceHost()).FontSize(11).TextColor(accentSoft)
						})
					})

					ui.Column(c).Gap(7).Children(func() {
						detailRow(c, "当前进度", record.ProgressLabel())
						detailRow(c, "最近更新", record.UpdatedAt.Format("2006-01-02 15:04"))
						detailRow(c, "创建时间", record.CreatedAt.Format("2006-01-02 15:04"))
					})

					ui.Text(c, historyMessage(record)).FontSize(11).LineHeight(1.5).TextColor(textSecondary)
					if a.historyError != "" {
						ui.Text(c, a.historyError).FontSize(11).TextColor(danger)
					}
				})
			})

			ui.Divider(c).Background(hairline)

			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "关闭").OnClick(func() { a.showHistory = false })
				ui.Button(c, "删除记录").TextColor(danger).Disabled(a.selectedID == "").OnClick(func() { a.pendingDelete = true })
				ui.Spacer(c)
				_, ok := a.selectedRecord()
				ui.PrimaryButton(c, "导出 EPUB").Disabled(!ok).OnClick(a.exportHistory)
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
	ui.Row(c).Children(func() {
		ui.Text(c, title).FontSize(11).TextColor(textMuted)
		ui.Spacer(c)
		ui.Text(c, value).FontSize(11).TextColor(textPrimary)
	})
}

func (a *app) chaptersDialog(c *ui.Context) {
	height := float32(200 + min(len(a.chapters), 6)*40)
	ui.DialogBase(c, &a.showChapters, func(backdrop, sheet ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.55))
		sheet.Padding(24).Radius(16).Background(raised).Border(1, hairlineLit).
			Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.45))
		ui.Column(c).Width(580).Height(height).Gap(16).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				ui.Text(c, "已下载章节").FontSize(19).Bold().TextColor(textPrimary)
				ui.Text(c, "删除后会从导出结果中排除，但不会回退下载进度").FontSize(11).TextColor(textMuted)
			})

			ui.Textf(c, "共 %d 章", len(a.chapters)).FontSize(11).TextColor(textMuted)

			ui.List(c, nil, len(a.chapters), func(i int) {
				chapter := a.chapters[i]
				ui.Row(c).Padding(8, 0).Height(40).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, chapter.Title).FontSize(12).TextColor(textPrimary).Grow(1)
					if !a.job.Busy() {
						ui.Text(c, "移除").FontSize(11).TextColor(textMuted).
							Padding(4, 6).OnClick(func() { a.removeChapter(chapter.Order) })
					}
				})
				ui.Divider(c).Background(hairline)
			}).Grow(1)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "关闭").OnClick(func() { a.showChapters = false })
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
