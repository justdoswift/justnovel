// JustNovel reads Chinese web novels from the sites you can open in a
// browser, and writes them as EPUB files for offline reading.
//
// The window pairs a status sidebar with a browser panel: open a catalog
// page there, solve the site's verification once, and JustNovel keeps the
// session's cookies to read the catalog and every chapter.
package main

import (
	"log"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"
)

func main() {
	app := newApp()

	// Checks for a newer release in the background and, from the app menu, on
	// demand. The window is native UI, so the native updater draws it too.
	mygo.Use(native.Plugin)

	mygo.App.WhenReady(func() {
		data, err := mygo.App.Path(mygo.PathUserData)
		if err == nil {
			app.history = NewHistory(filepath.Join(data, "history.json"))
		}

		mygo.App.SetMenu(mainMenu())

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "JustNovel",
			Width:           1240,
			Height:          800,
			MinWidth:        1080,
			MinHeight:       720,
			BackgroundColor: "#0a0d12",
			TitleBarStyle:   mygo.TitleBarHidden,
			Hidden:          true,
			Content:         ui.View(app.view),
		})
		app.win = win

		view, err := win.NewWebView(mygo.WebViewOptions{BackgroundColor: "#0a0d12"})
		if err != nil {
			log.Fatal(err)
		}
		app.session = newBrowserSession(win, view, app.source)
		// The browser comes forward only when a site demands a human.
		app.session.OnManual(func() {
			win.Update(func() {
				app.showBrowser = true
				app.awaiting = true
				app.logf(LogWarn, "需要人工验证", "在浏览器面板完成验证后会自动继续")
			})
		})
		win.Show()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// mainMenu is the app menu: about, check for updates, hide, quit. The update
// item opens the same window the background check does.
func mainMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "JustNovel", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			updater.MenuItem(),
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo},
			{Role: mygo.RoleRedo},
			mygo.Separator(),
			{Role: mygo.RoleCut},
			{Role: mygo.RoleCopy},
			{Role: mygo.RolePaste},
			{Role: mygo.RoleSelectAll},
		}},
		{Label: "窗口", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize},
			{Role: mygo.RoleZoom},
		}},
	})
}
