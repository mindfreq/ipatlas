package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"os"
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

// appSettingsFile is the name of the persisted user preferences.
const appSettingsFile = "settings.json"

// appSettings is the on-disk shape of the persisted settings.
type appSettings struct {
	Dark bool `json:"dark"`
}

// loadSettings restores the saved settings, if there are any, and returns
// where to keep them.
func loadSettings(a *appui.App) string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("settings: user data directory: %v", err)
		return ""
	}
	path := filepath.Join(dir, appSettingsFile)
	if data, err := os.ReadFile(path); err == nil {
		var s appSettings
		if json.Unmarshal(data, &s) == nil {
			a.SetThemeChoice(s.Dark)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("settings: read: %v", err)
	}
	return path
}

// saveSettings persists the user's theme choice. The write stays on the UI
// thread: the file is one line, so the click handler cannot stall.
func saveSettings(path string, s appSettings) {
	if path == "" {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("settings: write: %v", err)
	}
}

func main() {
	a := appui.New()
	mygo.App.WhenReady(func() {
		registerFont(fontRegular)
		registerFont(fontBold)

		// The packaged and dev builds carry the version from mygo.json;
		// a raw go run has none.
		a.SetVersion(mygo.App.Version())

		// Restore the saved theme before the first frame is drawn, then
		// save every flip of the rail's toggle.
		settingsPath := loadSettings(a)
		a.OnThemeChange(func(dark bool) {
			saveSettings(settingsPath, appSettings{Dark: dark})
		})

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
