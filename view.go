package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// The app's palette, a dark canvas with a warm accent.
var (
	colChrome       = ui.RGBA(255, 255, 255, 0.035)
	colSidebar      = ui.RGBA(255, 255, 255, 0.04)
	colStage        = ui.Hex("#14141a")
	colInset        = ui.RGBA(255, 255, 255, 0.025)
	colField        = ui.RGBA(0, 0, 0, 0.28)
	colBorder       = ui.RGBA(255, 255, 255, 0.09)
	colBorderStrong = ui.RGBA(255, 255, 255, 0.15)
	colTextPrimary  = ui.RGBA(255, 255, 255, 0.97)
	colTextSecond   = ui.RGBA(255, 255, 255, 0.72)
	colTextMuted    = ui.RGBA(255, 255, 255, 0.46)
	colAccent       = ui.Hex("#d97857")
	colAccentText   = ui.Hex("#eda684")
	colSuccess      = ui.Hex("#54d187")
	colWarning      = ui.Hex("#fba63b")
	colError        = ui.Hex("#f56b5e")
)

// theme is the app's dark look, close to the design the window draws.
var theme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#0b0b0f")
	t.Surface = ui.RGBA(255, 255, 255, 0.06)
	t.SurfaceHover = ui.RGBA(255, 255, 255, 0.10)
	t.SurfacePressed = ui.RGBA(255, 255, 255, 0.14)
	t.Border = ui.RGBA(255, 255, 255, 0.10)
	t.Text = colTextPrimary
	t.TextMuted = colTextMuted
	t.Accent = colAccent
	t.AccentHover = ui.Hex("#e2866a")
	t.AccentPressed = ui.Hex("#c26848")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = colError
	t.Warning = colWarning
	t.Success = colSuccess
	return t
}()

// view builds the window: the status sidebar beside the browser panel.
func (a *app) view(c *ui.Context) {
	c.SetTheme(theme)

	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		a.headerBar(c)

		ui.Row(c).Grow(1).Gap(16).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			a.stage(c)
		})
	})

	a.exportDialog(c)
	a.historyDialog(c)
	a.chaptersDialog(c)
	a.deleteAlert(c)
}

func (a *app) headerBar(c *ui.Context) {
	ui.Row(c).Padding(12, 18).Gap(8).Radius(18).
		Background(colChrome).Border(1, colBorder).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "JustNovel").FontSize(14).Bold().TextColor(colTextPrimary)
			ui.Text(c, "同步目录、下载章节、导出 EPUB").FontSize(11).TextColor(colTextMuted)
		})

		ui.Spacer(c)

		ui.Button(c, "打开目录").Disabled(!a.canParse()).OnClick(a.openSource)
		ui.Button(c, "同步会话").Disabled(!a.canParse()).OnClick(func() { go a.syncSession() })
		primaryButton(c, a.downloadButtonTitle(), a.canPause() || a.canResume() || (a.catalog != nil && !a.job.Busy()), a.toggleDownload)
		ui.Button(c, "导出文件").Disabled(!a.canExportSession()).OnClick(a.openExportDialog)
		ui.Button(c, "导出历史").OnClick(a.openHistory)
		if a.showBrowser {
			ui.Button(c, "隐藏浏览器").OnClick(func() { a.showBrowser = false })
		} else {
			ui.Button(c, "显示浏览器").OnClick(func() { a.showBrowser = true })
		}
	})
}

func primaryButton(c *ui.Context, label string, enabled bool, action func()) {
	ui.PrimaryButton(c, label).Disabled(!enabled).OnClick(action)
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
	ui.Column(c).Width(320).Gap(12).Children(func() {
		a.statusCard(c)
		if a.catalog != nil {
			a.catalogCard(c, *a.catalog)
		}
		if a.canExportSession() {
			a.sessionCard(c)
		}
		if a.lastSaved != "" {
			ui.Text(c, a.lastSaved).FontSize(11).TextColor(colTextMuted)
		}

		ui.Spacer(c)

		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "仅用于抓取你有权访问的公开页面内容。").FontSize(11).TextColor(colTextMuted)
			if a.history != nil {
				ui.Textf(c, "缓存 %s", humanBytes(a.history.ByteCount())).FontSize(10).TextColor(colTextMuted)
			}
		})
	})
}

func card(c *ui.Context, padding float32, fn func()) ui.Element {
	return ui.Column(c).Padding(padding).Gap(10).Radius(20).
		Background(colSidebar).Border(1, colBorder).Children(fn)
}

func (a *app) statusCard(c *ui.Context) {
	card(c, 20, func() {
		ui.Text(c, "当前状态").FontSize(10).TextColor(colTextMuted)
		ui.Text(c, a.job.Title()).FontSize(18).Bold().TextColor(colTextPrimary)
		if !a.job.Busy() || a.job.Phase == PhaseParsing {
			ui.Text(c, a.statusDetail()).FontSize(12).LineHeight(1.4).TextColor(colTextSecond)
		}

		if snap := a.job.Snap; snap != nil && (a.job.Phase == PhaseDownloading || a.job.Phase == PhasePaused) {
			ui.Progress(c, snap.Fraction()).TextColor(progressColor(a.job.Phase))
			ui.Textf(c, "已完成 %d / %d 章", snap.Completed, snap.Total).FontSize(11).TextColor(colTextSecond)
			ui.Textf(c, "正在处理：第 %d 章《%s》", snap.CurrentIndex, snap.CurrentTitle).FontSize(11).TextColor(colTextSecond)
			if snap.HasLast {
				ui.Textf(c, "最近完成：第 %d 章《%s》", snap.LastIndex, snap.LastTitle).FontSize(11).TextColor(colTextMuted)
			}
		}

		if a.awaiting {
			ui.Text(c, "检测到 Cloudflare 人工认证，请在浏览器面板完成验证。").FontSize(11).TextColor(colWarning)
		}

		ui.Divider(c).Background(colBorder)
		ui.Text(c, a.session.State().Status).FontSize(11).TextColor(colTextSecond)
		if err := a.session.State().LastError; err != "" {
			ui.Text(c, err).FontSize(11).TextColor(colError)
		}
	})
}

func progressColor(phase JobPhase) ui.Color {
	switch phase {
	case PhaseFinished:
		return colSuccess
	case PhaseFailed:
		return colError
	case PhasePaused, PhaseSessionRequired:
		return colWarning
	default:
		return colAccent
	}
}

func (a *app) catalogCard(c *ui.Context, catalog BookCatalog) {
	card(c, 16, func() {
		ui.Text(c, "目录摘要").FontSize(10).TextColor(colTextMuted)
		ui.Text(c, catalog.Title).FontSize(16).Bold().TextColor(colTextPrimary)
		if catalog.Author != "" && catalog.Author != "Unknown" {
			ui.Text(c, catalog.Author).FontSize(12).TextColor(colTextSecond)
		}
		ui.Textf(c, "%d 章", len(catalog.Chapters)).FontSize(11).Bold().TextColor(colAccentText)
	})
}

func (a *app) sessionCard(c *ui.Context) {
	card(c, 16, func() {
		ui.Text(c, "当前会话").FontSize(10).TextColor(colTextMuted)
		ui.Textf(c, "已下载 %d 章，可以先导出当前内容。", len(a.chapters)).FontSize(12).TextColor(colTextSecond)
		ui.PrimaryButton(c, "导出当前内容").OnClick(a.openExportDialog)
	})
}

func (a *app) stage(c *ui.Context) {
	ui.Column(c).Grow(1).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Padding(16).Gap(12).Radius(22).
			Background(colStage).Border(1, colBorderStrong).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "浏览器会话").FontSize(18).Bold().TextColor(colTextPrimary)
				ui.Spacer(c)
				status := a.session.State()
				color := colTextMuted
				if status.Loading {
					color = colAccentText
				}
				ui.Text(c, status.Status).FontSize(11).TextColor(color).TextAlign(ui.End)
			})

			a.sourceBar(c)

			if a.source.CatalogURL(a.urlText) == nil {
				ui.Text(c, a.source.InvalidText).FontSize(11).TextColor(colError)
			}

			if a.showBrowser && a.session.view != nil {
				ui.WebView(c, a.session.view).Grow(1).Radius(14).Border(1, colBorderStrong)
			}
		})

		if a.showRail() {
			a.rail(c)
		}
	})
}

func (a *app) sourceBar(c *ui.Context) {
	ui.Row(c).Padding(6, 12).Gap(8).Radius(14).
		Background(colField).Border(1, colBorder).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "链接").FontSize(11).TextColor(colTextMuted)
		ui.TextInput(c, &a.urlText).Placeholder(a.source.Placeholder).Grow(1)
		ui.Divider(c).Width(1).Height(20).Background(colBorder)

		names := make([]string, len(Sources))
		for i, source := range Sources {
			names[i] = source.Name
		}
		chosen := a.source.Name
		ui.Select(c, &chosen, names).Width(120).OnChange(func() {
			for _, source := range Sources {
				if source.Name == chosen {
					a.selectSource(source.Kind)
				}
			}
		})
	})
}

func (a *app) showRail() bool {
	switch a.job.Phase {
	case PhasePaused, PhaseFinished, PhaseFailed:
		return true
	}
	return len(a.chapters) > 0
}

func (a *app) rail(c *ui.Context) {
	ui.Row(c).Padding(12, 18).Gap(10).Radius(18).
		Background(colChrome).Border(1, colBorder).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Children(func() {
			ui.Text(c, a.railTitle()).FontSize(11).Bold().TextColor(colTextPrimary)
			if message := a.railMessage(); message != "" {
				ui.Text(c, message).FontSize(11).TextColor(colTextSecond)
			}
		})

		if a.canResume() {
			ui.Button(c, "继续当前下载").OnClick(a.resumeDownload)
		}
		if a.job.Phase == PhaseFailed && !a.running && a.catalog != nil && a.next < len(a.catalog.Chapters) {
			ui.PrimaryButton(c, "跳过当前章").OnClick(a.skipChapter)
		}
		if a.canExportSession() {
			ui.Button(c, "导出已完成内容").OnClick(a.openExportDialog)
		}
		if len(a.chapters) > 0 {
			ui.Button(c, "查看已下载章节").OnClick(func() { a.showChapters = true })
		}
		if a.job.Phase == PhasePaused || a.job.Phase == PhaseFailed {
			ui.Button(c, "停止并清空").OnClick(a.stopDownload)
		}
	})
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
			return fmt.Sprintf("已经完成 %d / %d 章，可继续、跳过当前章，或先导出当前内容。", snap.Completed, snap.Total)
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
	ui.Modal(c, &a.showExport, func() {
		ui.Column(c).Width(560).Gap(14).Children(func() {
			ui.Text(c, "导出设置").FontSize(18).Bold().TextColor(colTextPrimary)
			ui.Text(c, "这里不会改变抓取逻辑，只会改变导出的排版与附加信息。").FontSize(11).TextColor(colTextMuted)

			ui.Text(c, "作者信息").FontSize(11).Bold().TextColor(colTextSecond)
			ui.TextInput(c, &a.draft.Author).Placeholder("作者")

			ui.Divider(c).Background(colBorder)

			ui.Text(c, "正文字体").FontSize(11).Bold().TextColor(colTextSecond)
			fonts := make([]string, len(FontPresets))
			for i, preset := range FontPresets {
				fonts[i] = preset.DisplayName()
			}
			if a.exportFont == "" {
				a.exportFont = a.draft.BodyFont.DisplayName()
			}
			ui.Select(c, &a.exportFont, fonts)

			ui.Text(c, "首页内容").FontSize(11).Bold().TextColor(colTextSecond)
			ui.TextInput(c, &a.draft.FrontText).Multiline(true).MinHeight(120)
			ui.Checkbox(c, &a.draft.ShowSource, "首页显示来源链接")

			ui.Divider(c).Background(colBorder)

			ui.Text(c, "封面").FontSize(11).Bold().TextColor(colTextSecond)
			ui.Text(c, "可选。支持 PNG、JPEG 和 WebP。").FontSize(11).TextColor(colTextMuted)

			ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
				if a.coverPreview != nil {
					ui.Image(c, a.coverPreview).Size(150, 200).Radius(14).Border(1, colBorderStrong)
				} else {
					ui.Column(c).Size(150, 200).Radius(14).Background(colInset).Border(1, colBorder).
						AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Text(c, "未设置封面").FontSize(11).TextColor(colTextMuted)
					})
				}
				ui.Column(c).Gap(10).Children(func() {
					ui.Text(c, "封面会直接写入导出文件，也会显示在历史记录里。").FontSize(11).TextColor(colTextSecond)
					label := "选择封面"
					if len(a.draft.CoverImage) > 0 {
						label = "更换封面"
					}
					ui.PrimaryButton(c, label).OnClick(a.importCover)
					if len(a.draft.CoverImage) > 0 {
						ui.Button(c, "移除封面").OnClick(a.removeCover)
					}
				})
			})

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.showExport = false })
				ui.PrimaryButton(c, "导出文件").OnClick(a.confirmExport)
			})
		})
	})
}

func (a *app) historyDialog(c *ui.Context) {
	ui.Modal(c, &a.showHistory, func() {
		ui.Column(c).Width(760).Height(480).Gap(12).Children(func() {
			ui.Text(c, "下载历史").FontSize(18).Bold().TextColor(colTextPrimary)
			ui.Text(c, "这里保存可再次导出的 EPUB 快照。").FontSize(11).TextColor(colTextMuted)

			ui.Row(c).Grow(1).Gap(14).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(250).Padding(8).Gap(4).Radius(14).Background(colInset).Children(func() {
					if len(a.historyList) == 0 {
						ui.Text(c, "还没有记录").FontSize(12).TextColor(colTextSecond)
						ui.Text(c, "开始一次下载后，这里会保存可再次导出的 EPUB 快照。").FontSize(11).TextColor(colTextMuted)
						return
					}
					ui.List(c, nil, len(a.historyList), func(i int) {
						record := a.historyList[i]
						row := ui.Column(c).Padding(10).Gap(4).Radius(10)
						if record.ID == a.selectedID {
							row.Background(colStage).Border(1, colBorderStrong)
						}
						row.OnClick(func() { a.selectHistory(record.ID) })
						ui.Text(c, record.Title).FontSize(12).Bold().TextColor(colTextPrimary)
						ui.Textf(c, "%s · %s", record.ProgressLabel(), record.UpdatedAt.Format("2006-01-02")).FontSize(10).TextColor(colTextMuted)
					}).Grow(1)
				})

				ui.Column(c).Grow(1).Padding(14).Gap(12).Radius(14).Background(colInset).Children(func() {
					record, ok := a.selectedRecord()
					if !ok {
						ui.Spacer(c)
						ui.Text(c, "还没有下载历史").FontSize(15).Bold().TextColor(colTextPrimary).TextAlign(ui.Center)
						ui.Spacer(c)
						return
					}

					ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
						if a.historyCover != nil {
							ui.Image(c, a.historyCover).Size(110, 150).Radius(12).Border(1, colBorderStrong)
						}
						ui.Column(c).Gap(6).Grow(1).Children(func() {
							ui.Text(c, record.Title).FontSize(16).Bold().TextColor(colTextPrimary)
							if record.Author != "" && record.Author != "Unknown" {
								ui.Text(c, record.Author).FontSize(12).TextColor(colTextSecond)
							}
							ui.Textf(c, "%s · %s", completeLabel(record), record.SourceHost()).FontSize(11).TextColor(colAccentText)
						})
					})

					detailRow(c, "导出内容", exportScope(record))
					detailRow(c, "当前进度", record.ProgressLabel())
					detailRow(c, "最近更新", record.UpdatedAt.Format("2006-01-02 15:04"))
					detailRow(c, "创建时间", record.CreatedAt.Format("2006-01-02 15:04"))

					ui.Text(c, historyMessage(record)).FontSize(11).TextColor(colTextSecond)
					if a.historyError != "" {
						ui.Text(c, a.historyError).FontSize(11).TextColor(colError)
					}
				})
			})

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Button(c, "关闭").OnClick(func() { a.showHistory = false })
				ui.Button(c, "删除记录").Disabled(a.selectedID == "").OnClick(func() { a.pendingDelete = true })
				ui.Spacer(c)
				_, ok := a.selectedRecord()
				ui.PrimaryButton(c, "导出文件").Disabled(!ok).OnClick(a.exportHistory)
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

func exportScope(record HistoryRecord) string {
	if len(record.Chapters) == 0 {
		return "暂无可导出章节"
	}
	return completeLabel(record)
}

func historyMessage(record HistoryRecord) string {
	if len(record.Chapters) == 0 {
		return "这条记录还没有保存任何章节内容，因此暂时不能导出文件。"
	}
	if !record.Complete() {
		return "此记录保存了当时已下载的内容。导出时会生成一份部分 EPUB。"
	}
	return "此记录保存了完整下载内容，你可以直接再次导出文件。"
}

func detailRow(c *ui.Context, title, value string) {
	ui.Row(c).Children(func() {
		ui.Text(c, title).FontSize(12).TextColor(colTextSecond)
		ui.Spacer(c)
		ui.Text(c, value).FontSize(12).Bold().TextColor(colTextPrimary)
	})
}

func (a *app) chaptersDialog(c *ui.Context) {
	ui.Modal(c, &a.showChapters, func() {
		ui.Column(c).Width(560).Height(420).Gap(12).Children(func() {
			ui.Text(c, "已下载章节").FontSize(18).Bold().TextColor(colTextPrimary)
			ui.Text(c, "删除章节后会从导出结果中排除，但不会回退当前下载进度。").FontSize(11).TextColor(colTextMuted)
			ui.Textf(c, "共 %d 章", len(a.chapters)).FontSize(11).TextColor(colTextMuted)

			ui.List(c, nil, len(a.chapters), func(i int) {
				chapter := a.chapters[i]
				ui.Row(c).Padding(8, 12).Gap(10).Radius(10).Background(colInset).AlignItems(ui.Center).Children(func() {
					ui.Textf(c, "%03d", chapter.Order).FontSize(10).TextColor(colTextMuted).Width(34)
					ui.Text(c, chapter.Title).FontSize(12).TextColor(colTextPrimary).Grow(1)
					ui.Button(c, "删除").Disabled(a.job.Busy()).OnClick(func() { a.removeChapter(chapter.Order) })
				})
			}).Grow(1)

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "关闭").OnClick(func() { a.showChapters = false })
			})
		})
	})
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
