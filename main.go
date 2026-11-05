package main

import (
	"log"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	appui "ipatlas/ui"
)

func main() {
	a := appui.New()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
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
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
