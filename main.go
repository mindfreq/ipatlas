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

//go:embed resources/fonts/IBMPlexSansArabic-Regular.ttf
var fontRegular []byte

//go:embed resources/fonts/IBMPlexSansArabic-Bold.ttf
var fontBold []byte

// appFontFamily is the family the About fonts register under and the theme
// uses.
const appFontFamily = "IBM Plex Sans Arabic"

func main() {
	a := appui.New()
	mygo.App.WhenReady(func() {
		registerFont(fontRegular)
		registerFont(fontBold)

		// The packaged and dev builds carry the version from mygo.json;
		// a raw go run has none.
		a.SetVersion(mygo.App.Version())

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
			Title:     "IPAtlas",
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

// registerFont makes one bundled IBM Plex Sans Arabic face available to the
// app's text. A failed bundle only means the system font stays in use.
func registerFont(data []byte) {
	if err := myui.RegisterFont(data, appFontFamily); err != nil {
		log.Printf("font: register %s face: %v", appFontFamily, err)
	}
}
