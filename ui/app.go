// Package ui builds the IPatlas native interface. It calls the domain
// packages for all IP math; MyGo types never leave this package.
package ui

import (
	myui "github.com/egoist/mygo/ui"
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

// App is the state the window shows. Its View builds the interface from it,
// on the main thread, whenever the window needs a frame.
type App struct {
	tab  int
	ipv4 ipv4State
	vlsm vlsmState
	ipv6 ipv6State
}

// New returns an application with the IPv4 tab selected.
func New() *App {
	a := &App{}
	a.vlsm.addRow()
	return a
}

// View builds the application shell and the selected section.
func (a *App) View(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Fill().Padding(24).Gap(16).Children(func() {
		myui.Row(c).AlignItems(myui.Center).Gap(12).Children(func() {
			myui.Text(c, "IPatlas").FontSize(26).Bold()
			myui.Text(c, "Subnet calculator, IPAM and live scan").TextColor(t.TextMuted)
		})
		myui.Tabs(c, &a.tab, tabLabels...)
		myui.Divider(c)
		a.sectionView(c)
	})
}

func (a *App) sectionView(c *myui.Context) {
	if a.tab < 0 || a.tab >= len(sections) {
		a.tab = 0
	}
	if a.tab == 0 {
		a.ipv4View(c)
		return
	}
	if a.tab == 1 {
		a.vlsmView(c)
		return
	}
	if a.tab == 2 {
		a.ipv6View(c)
		return
	}

	s := sections[a.tab]
	myui.Column(c).Fill().Gap(8).Children(func() {
		myui.Text(c, s.title).FontSize(20).Bold()
		myui.Text(c, s.blurb).TextColor(c.Theme().TextMuted)
	})
}
