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
	"github.com/egoist/mygo/ui"
)

func main() {
	app := newApp()

	mygo.App.WhenReady(func() {
		data, err := mygo.App.Path(mygo.PathUserData)
		if err == nil {
			app.history = NewHistory(filepath.Join(data, "history.json"))
		}

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "JustNovel",
			Width:           1280,
			Height:          820,
			MinWidth:        1180,
			MinHeight:       760,
			BackgroundColor: "#0b0b0f",
			Hidden:          true,
			Content:         ui.View(app.view),
		})
		app.win = win

		view, err := win.NewWebView(mygo.WebViewOptions{BackgroundColor: "#14141a"})
		if err != nil {
			log.Fatal(err)
		}
		app.session = newBrowserSession(win, view, app.source)
		app.session.Open(app.source.Placeholder)
		win.Show()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
