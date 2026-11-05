package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var tabLabels = []string{"IPv4", "VLSM", "IPv6", "IPAM", "Scan"}

type section struct {
	title string
	blurb string
}

var sections = []section{
	{
		title: "IPv4 calculator",
		blurb: "Enter a CIDR block to see its network, broadcast, masks and usable host range.",
	},
	{
		title: "VLSM planner",
		blurb: "Allocate non-overlapping subnets from a parent network, largest department first.",
	},
	{
		title: "IPv6 tools",
		blurb: "Compress, expand and classify addresses, then page through large prefix splits.",
	},
	{
		title: "IP address management",
		blurb: "Save calculated networks locally, label hosts and see which addresses are in use.",
	},
	{
		title: "Live scan",
		blurb: "Probe a network with a bounded worker pool and colour every address by its state.",
	},
}

// app is the state the window shows. Its view builds the interface from it,
// on the main thread, whenever the window needs a frame.
type app struct {
	tab int
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(16).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "IPatlas").FontSize(26).Bold()
			ui.Text(c, "Subnet calculator, IPAM and live scan").TextColor(t.TextMuted)
		})
		ui.Tabs(c, &a.tab, tabLabels...)
		ui.Divider(c)
		a.section(c)
	})
}

func (a *app) section(c *ui.Context) {
	if a.tab < 0 || a.tab >= len(sections) {
		a.tab = 0
	}
	s := sections[a.tab]
	ui.Column(c).Fill().Gap(8).Children(func() {
		ui.Text(c, s.title).FontSize(20).Bold()
		ui.Text(c, s.blurb).TextColor(c.Theme().TextMuted)
	})
}

func main() {
	a := &app{}
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
			Content: ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
