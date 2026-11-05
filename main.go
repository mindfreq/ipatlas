package main

import (
	_ "embed"
	"log"
	"path/filepath"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
	appui "ipatlas/ui"
)

//go:embed resources/icon.png
var iconPNG []byte

func main() {
	mygo.App.SetName("IPatlas")
	mygo.App.SetVersion("0.1.0")

	a := appui.New()
	mygo.App.WhenReady(func() {
		var store *ipam.Store
		if dir, err := mygo.App.Path(mygo.PathUserData); err != nil {
			log.Printf("ipam: user data directory: %v", err)
		} else if opened, err := ipam.Open(filepath.Join(dir, "ipatlas.db")); err != nil {
			log.Printf("ipam: open: %v", err)
		} else {
			store = opened
			a.SetStore(store)
		}

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "IPatlas",
			Width:     1100,
			Height:    720,
			MinWidth:  820,
			MinHeight: 560,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: myui.View(a.View),
		})
		if err := win.SetIcon(iconPNG); err != nil {
			log.Printf("window icon: %v", err)
		}

		mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
			a.StopScan()
			if store != nil {
				if err := store.Close(); err != nil {
					log.Printf("ipam: close: %v", err)
				}
			}
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
