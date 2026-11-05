// Package ui builds the IPatlas native interface. It calls the domain
// packages for all IP math; MyGo types never leave this package.
package ui

import (
	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
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
	nav  string
	ipv4 ipv4State
	vlsm vlsmState
	ipv6 ipv6State
	ipam ipamState
	scan scanState
}

// New returns an application without an IPAM database. The app attaches one
// with SetStore once the user data directory is available.
func New() *App {
	return NewWithStore(nil)
}

// NewWithStore returns an application backed by store, which may be nil.
func NewWithStore(store *ipam.Store) *App {
	a := &App{nav: tabLabels[0]}
	a.vlsm.addRow()
	a.ipam.attach(store)
	return a
}

// SetStore attaches the IPAM database and loads its saved networks.
func (a *App) SetStore(store *ipam.Store) {
	a.ipam.attach(store)
}

// View builds the application shell and the selected section.
func (a *App) View(c *myui.Context) {
	t := appTheme(c)
	if a.tab < 0 || a.tab >= len(tabLabels) {
		a.tab = 0
	}
	if a.nav == "" {
		a.nav = tabLabels[a.tab]
	}

	w, _ := c.Size()
	pad := pagePadding(w)
	myui.Row(c).Fill().Children(func() {
		a.navRail(c, t)
		myui.Column(c).Fill().Grow(1).MinWidth(0).Padding(pad, pad, pad, pad).Gap(spaceXL).Children(func() {
			a.pageHeader(c, t)
			a.sectionView(c)
		})
	})
}

// navRail is the persistent navigation beside every tool.
func (a *App) navRail(c *myui.Context, t *myui.Theme) {
	myui.Column(c).
		Width(navWidth).
		FillHeight().
		Padding(14).
		Gap(spaceL).
		Background(t.Surface).
		BorderWidth(0, 1, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			myui.Row(c).Gap(10).AlignItems(myui.Center).Padding(2, 2).Children(func() {
				mark := myui.Box(c).Size(36, 36).Radius(11).Background(t.Accent)
				mark.Children(func() {
					myui.Column(c).Fill().Center().Children(func() {
						myui.Text(c, "IP").FontSize(12.5).FontWeight(750).TextColor(t.AccentText)
					})
				})
				myui.Column(c).Gap(0).Children(func() {
					myui.Text(c, "IPatlas").FontSize(16).FontWeight(650)
					myui.Text(c, "Subnet toolkit").FontSize(11).TextColor(t.TextMuted)
				})
			})

			myui.Sidebar(c, &a.nav, func() {
				myui.SidebarSection(c, "Tools", nil, func() {
					for i, label := range tabLabels {
						myui.SidebarItem(c, label, navIcons[i], label)
					}
				})
			}).Width(navWidth - 28).Grow(1).OnChange(func() {
				a.selectTab(a.nav)
			})

			myui.Column(c).Gap(2).Padding(8, 4, 0, 4).Children(func() {
				myui.Text(c, "Native UI").FontSize(10.5).FontWeight(600).TextColor(t.TextMuted)
				myui.Text(c, "Data stays on this device").FontSize(10.5).TextColor(t.TextMuted)
			})
		})
}

func (a *App) selectTab(label string) {
	for i, candidate := range tabLabels {
		if candidate == label {
			a.tab = i
			return
		}
	}
}

func (a *App) pageHeader(c *myui.Context, t *myui.Theme) {
	s := sections[a.tab]
	myui.Column(c).Gap(3).Children(func() {
		eyebrow(c, "IPATLAS WORKSPACE")
		myui.Text(c, s.title).FontSize(25).FontWeight(650)
		myui.Text(c, s.blurb).FontSize(13).TextColor(t.TextMuted).MaxWidth(720)
	})
}

func (a *App) sectionView(c *myui.Context) {
	switch a.tab {
	case 0:
		a.ipv4View(c)
	case 1:
		a.vlsmView(c)
	case 2:
		a.ipv6View(c)
	case 3:
		a.ipamView(c)
	case 4:
		a.scanView(c)
	}
}
