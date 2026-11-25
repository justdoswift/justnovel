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
	// The window is drawn in a light native appearance, so the sidebar's
	// material, the traffic lights and the white content agree even when the
	// desktop itself is in dark mode.
	mygo.Theme.SetSource(mygo.ThemeLight)

	// Checks for a newer release in the background and, from the app menu, on
	// demand. The window is native UI, so the native updater draws it too.
	mygo.Use(native.Plugin)

	mygo.App.WhenReady(func() {
		var history *History
		if data, err := mygo.App.Path(mygo.PathUserData); err == nil {
			history = NewHistory(filepath.Join(data, "history"))
		}

		mygo.App.SetMenu(mainMenu())

		// The window is built before the session, because the session needs
		// the window to post its updates through. The view is not called until
		// the window draws, by which point both are set.
		//
		window := &app{history: history}
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "JustNovel",
			Width:     1180,
			Height:    760,
			MinWidth:  980,
			MinHeight: 620,
			// The view draws its own title bar under the traffic lights, and
			// the sidebar sits on the window's material.
			TitleBarStyle:   mygo.TitleBarHiddenInset,
			Vibrancy:        mygo.VibrancySidebar,
			BackgroundColor: "#ffffff",
			Hidden:          true,
			Content:         ui.View(window.view),
		})
		window.win = win

		view, err := win.NewWebView(mygo.WebViewOptions{BackgroundColor: "#ffffff"})
		if err != nil {
			log.Fatal(err)
		}

		source := SourceFor(SourceShuba69)
		panel := newBrowserPanel(win, view, source)
		session := NewSession(win, panel, history, source)
		window.session = session
		window.panel = panel

		// The browser comes forward only when a site demands a human.
		panel.OnManual(func() {
			session.update(func() {
				window.showBrowser = true
				session.logfLocked(LogWarn, "需要人工验证", "在浏览器面板完成验证后会自动继续")
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
