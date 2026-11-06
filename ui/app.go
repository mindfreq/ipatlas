// Package ui builds the IPAtlas native interface. It calls the domain
// packages for all IP math; MyGo types never leave this package.
package ui

import (
	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
)

var tabLabels = []string{"IPv4", "VLSM", "IPv6", "IPAM", "Scan", "About"}

// Rail geometry. The logo sits at the same place in both modes, and the
// small toggle sits at its right: after the name when open, directly next to
// it when collapsed.
const (
	railPad    = 12
	railItem   = 36 // logo and tab buttons height
	railGap    = 4  // logo-to-toggle gap when collapsed
	toggleSize = 24 // hamburger button

	// collapsed width = padding + logo + gap + toggle + padding
	navCollapsedWidth = railPad*2 + railItem + railGap + toggleSize // 88
	// width of the content area inside the collapsed rail
	railInner = navCollapsedWidth - railPad*2 // 64
)

// menuIcon is the three-line "hamburger" icon of the toggle button.
var menuIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#ffffff" stroke-width="2" stroke-linecap="round"><path d="M5 7h14M5 12h14M5 17h14"/></svg>`))

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
	{
		title: "About IPAtlas",
		blurb: "What this toolkit is, how it treats your data, and what it is built with.",
	},
}

// App is the state the window shows. Its View builds the interface from it,
// on the main thread, whenever the window needs a frame.
type App struct {
	tab          int
	nav          string
	navCollapsed bool   // false = full sidebar, true = icon-only rail
	version      string // application version from mygo.json, "" in raw go runs
	ipv4         ipv4State
	vlsm         vlsmState
	ipv6         ipv6State
	ipam         ipamState
	scan         scanState
}

// SetVersion records the application version the About page displays.
func (a *App) SetVersion(v string) {
	a.version = v
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

// navToggle is the small hamburger button, to the right of the logo.
func (a *App) navToggle(c *myui.Context) {
	tip := "Collapse sidebar"
	if a.navCollapsed {
		tip = "Expand sidebar"
	}
	btn := myui.Button(c, "").
		Key("nav-toggle").
		Size(toggleSize, toggleSize).
		Padding(0).
		Radius(8).
		Tooltip(tip).
		Label(tip).
		Children(func() {
			myui.Column(c).Fill().Center().Children(func() {
				myui.Icon(c, menuIcon).Size(20, 20)
			})
		})
	if btn.Clicked() {
		a.navCollapsed = !a.navCollapsed
	}
}

// logoMark is the "IP" badge, always the top-left element of the rail.
func (a *App) logoMark(c *myui.Context, t *myui.Theme) {
	myui.Box(c).Size(railItem, railItem).Radius(11).Background(t.Accent).Children(func() {
		myui.Column(c).Fill().Center().Children(func() {
			myui.Text(c, "IP").FontSize(12.5).FontWeight(750).TextColor(t.AccentText)
		})
	})
}

// navIconButton draws one icon-only entry of the collapsed rail.
func (a *App) navIconButton(c *myui.Context, i int) {
	label := tabLabels[i]

	var btn myui.Element
	if a.tab == i {
		btn = myui.PrimaryButton(c, "")
	} else {
		btn = myui.Button(c, "")
	}
	btn = btn.Key(label).Size(railInner, railItem).Tooltip(label).Label(label).Children(func() {
		myui.Column(c).Fill().Center().Children(func() {
			myui.Icon(c, navIcons[i]).Size(18, 18)
		})
	})
	if btn.Clicked() {
		a.selectTab(label)
	}
}

// navRail is the persistent navigation beside every tool.
func (a *App) navRail(c *myui.Context, t *myui.Theme) {
	width := float32(navWidth)
	if a.navCollapsed {
		width = navCollapsedWidth
	}

	myui.Column(c).
		Width(width).
		FillHeight().
		Padding(railPad).
		Gap(spaceL).
		Background(t.Surface).
		BorderWidth(0, 1, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			// Top row: logo first (top-left), small toggle on its right.
			gap := float32(10)
			if a.navCollapsed {
				gap = railGap
			}
			myui.Row(c).Gap(gap).AlignItems(myui.Center).Children(func() {
				a.logoMark(c, t)
				if !a.navCollapsed {
					myui.Column(c).Gap(0).Grow(1).Children(func() {
						myui.Text(c, "IPAtlas").FontSize(16).FontWeight(650)
						myui.Text(c, "Subnet toolkit").FontSize(11).TextColor(t.TextMuted)
					})
				}
				a.navToggle(c)
			})

			// Navigation: icons only when collapsed, full sidebar when open.
			if a.navCollapsed {
				myui.Column(c).Gap(6).Grow(1).Children(func() {
					for i := range tabLabels {
						a.navIconButton(c, i)
					}
				})
				return
			}

			myui.Sidebar(c, &a.nav, func() {
				myui.SidebarSection(c, "Tools", nil, func() {
					for i, label := range tabLabels {
						myui.SidebarItem(c, label, navIcons[i], label)
					}
				})
			}).Width(navWidth - railPad*2).Grow(1).OnChange(func() {
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
			a.nav = label // keep the Sidebar selection in sync
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
	case 5:
		a.aboutView(c)
	}
}
