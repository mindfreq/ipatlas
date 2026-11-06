package ui

import (
	myui "github.com/egoist/mygo/ui"
)

// The application's design tokens. Spacing follows a 4-point scale, cards
// share one radius, and the theme is a restrained indigo-on-neutral palette
// that keeps the native controls and the system's light/dark appearance.
const (
	navWidth      = 232
	cardRadius    = 16
	controlRadius = 10
	pagePadWide   = 30
	pagePadNarrow = 20
	spaceS        = 8
	spaceM        = 12
	spaceL        = 16
	spaceXL       = 24
	gridCellWidth = 156
)

// navIcons are parsed once, never in a frame.
var navIcons = []*myui.SVG{
	myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="4"/><path d="M7.5 8h9M7.5 12h9M7.5 16h5"/></svg>`)),
	myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></svg>`)),
	myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.4 2.5 3.6 5.6 3.6 9S14.4 18.5 12 21c-2.4-2.5-3.6-5.6-3.6-9S9.6 5.5 12 3Z"/></svg>`)),
	myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5" rx="7" ry="3"/><path d="M5 5v14c0 1.7 3.1 3 7 3s7-1.3 7-3V5"/><path d="M5 12c0 1.7 3.1 3 7 3s7-1.3 7-3"/></svg>`)),
	myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="4.5"/><circle cx="12" cy="12" r="1" fill="currentColor"/><path d="m12 12 6-4"/></svg>`)),
	myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/></svg>`)),
}

// appTheme derives the app's palette from the appearance in dark, which is
// the system's until the user toggles it with the rail's theme button. MyGo
// documents theming as changing a copy of LightTheme or DarkTheme, so the
// copy is applied once per frame; the control set stays native.
func appTheme(c *myui.Context, dark bool) *myui.Theme {
	base := myui.LightTheme()
	if dark {
		base = myui.DarkTheme()
	}
	base.Radius = controlRadius
	base.Spacing = 4
	base.FontSize = 14
	// The bundled IBM Plex Sans Arabic face; glyphs it lacks fall back to
	// the system's choice.
	base.Font = "IBM Plex Sans Arabic"

	if base.Dark {
		base.Background = myui.Hex("#0f1115")
		base.Surface = myui.Hex("#181b21")
		base.SurfaceHover = myui.Hex("#20242b")
		base.SurfacePressed = myui.Hex("#272c34")
		base.Border = myui.Hex("#2b303a")
		base.Text = myui.Hex("#f3f4f6")
		base.TextMuted = myui.Hex("#9aa1ac")
		base.Accent = myui.Hex("#6366f1")
		base.AccentHover = myui.Hex("#818cf8")
		base.AccentPressed = myui.Hex("#4f46e5")
		base.AccentText = myui.Hex("#ffffff")
		base.Danger = myui.Hex("#f87171")
		base.Warning = myui.Hex("#fbbf24")
		base.Success = myui.Hex("#34d399")
		base.Selection = myui.RGBA(99, 102, 241, 0.35)
		base.Focus = myui.RGBA(129, 140, 248, 0.6)
		base.Scrollbar = myui.RGBA(243, 244, 246, 0.25)
	} else {
		base.Background = myui.Hex("#f6f7f9")
		base.Surface = myui.Hex("#ffffff")
		base.SurfaceHover = myui.Hex("#f1f2f5")
		base.SurfacePressed = myui.Hex("#e9ebef")
		base.Border = myui.Hex("#e2e5ea")
		base.Text = myui.Hex("#111827")
		base.TextMuted = myui.Hex("#6b7280")
		base.Accent = myui.Hex("#4f46e5")
		base.AccentHover = myui.Hex("#4338ca")
		base.AccentPressed = myui.Hex("#3730a3")
		base.AccentText = myui.Hex("#ffffff")
		base.Danger = myui.Hex("#dc2626")
		base.Warning = myui.Hex("#d97706")
		base.Success = myui.Hex("#059669")
		base.Selection = myui.RGBA(79, 70, 229, 0.2)
		base.Focus = myui.RGBA(79, 70, 229, 0.5)
		base.Scrollbar = myui.RGBA(17, 24, 39, 0.25)
	}
	c.SetTheme(base)
	return base
}

// pagePadding adapts the page gutter to the window width.
func pagePadding(width float32) float32 {
	if width < 960 {
		return pagePadNarrow
	}
	return pagePadWide
}

// contentWidth is the room left to a page beside the navigation rail and its
// gutters.
func contentWidth(c *myui.Context) float32 {
	w, _ := c.Size()
	return max(w-navWidth-2*pagePadding(w), 0)
}

// columnsFor picks a responsive grid column count from the room left.
func columnsFor(c *myui.Context, minCell float32, maxColumns int) int {
	n := int(contentWidth(c) / minCell)
	if n < 1 {
		n = 1
	}
	if n > maxColumns {
		n = maxColumns
	}
	return n
}

// shadow is the subtle elevation under cards.
func shadow(t *myui.Theme) myui.Color {
	if t.Dark {
		return myui.RGBA(0, 0, 0, 0.35)
	}
	return myui.RGBA(17, 24, 39, 0.05)
}

// card groups related content on a raised surface.
func card(c *myui.Context, fn func()) myui.Element {
	t := c.Theme()
	return myui.Column(c).
		Padding(18).
		Gap(spaceM).
		Background(t.Surface).
		Border(1, t.Border).
		Radius(cardRadius).
		Shadow(0, 1, 4, 0, shadow(t)).
		Children(fn)
}

// cardHeader lays out one card title: a title with an optional description on
// the left and actions (pills, buttons) right-aligned on the same row. Every
// card header goes through this so titles, sizes and spacing stay identical.
func cardHeader(c *myui.Context, title, description string, actions func()) {
	t := c.Theme()
	myui.Row(c).Gap(spaceM).AlignItems(myui.Center).Wrap().Children(func() {
		myui.Column(c).Gap(2).Children(func() {
			myui.Text(c, title).FontSize(15).FontWeight(650)
			if description != "" {
				myui.Text(c, description).FontSize(12).TextColor(t.TextMuted)
			}
		})
		if actions != nil {
			myui.Spacer(c)
			actions()
		}
	})
}

// eyebrow is a small uppercase label above a page title.
func eyebrow(c *myui.Context, text string) {
	t := c.Theme()
	myui.Text(c, text).
		FontSize(10.5).
		FontWeight(700).
		LetterSpacing(1.3).
		TextColor(t.TextMuted)
}

// pill is a compact status or metadata token.
func pill(c *myui.Context, text string, color myui.Color) myui.Element {
	e := myui.Row(c).
		AlignItems(myui.Center).
		Padding(2, 8).
		Radius(999).
		Background(color.Alpha(0.12)).
		Shrink(0)
	e.Children(func() {
		myui.Text(c, text).FontSize(11).FontWeight(600).TextColor(color).SingleLine()
	})
	return e
}

// alert shows a validation or status message in a tinted block.
func alert(c *myui.Context, message string, color myui.Color) myui.Element {
	e := myui.Row(c).
		Gap(spaceS).
		AlignItems(myui.Center).
		Padding(10, 12).
		Background(color.Alpha(0.09)).
		Border(1, color.Alpha(0.22)).
		Radius(controlRadius)
	e.Children(func() {
		myui.Box(c).Width(7).Height(7).Radius(999).Background(color).Shrink(0)
		myui.Text(c, message).FontSize(12.5).TextColor(color).Selectable().Grow(1)
	})
	return e
}

// statTile is a labelled number for summaries.
func statTile(c *myui.Context, label, value string, color myui.Color) myui.Element {
	t := c.Theme()
	e := myui.Column(c).
		Gap(2).
		Padding(10, 12).
		Grow(1).
		Background(t.Background).
		Border(1, t.Border).
		Radius(controlRadius)
	e.Children(func() {
		myui.Text(c, label).FontSize(10.5).FontWeight(600).TextColor(t.TextMuted).SingleLine()
		myui.Text(c, value).FontSize(19).FontWeight(650).TextColor(color)
	})
	return e
}

// sectionLabel is a quiet heading inside a card.
func sectionLabel(c *myui.Context, text string) {
	t := c.Theme()
	myui.Text(c, text).FontSize(11).FontWeight(700).LetterSpacing(0.8).TextColor(t.TextMuted)
}

// inset is a recessed panel inside a card, one step quieter than a card.
func inset(c *myui.Context, fn func()) myui.Element {
	t := c.Theme()
	e := myui.Column(c).
		Gap(spaceS).
		Padding(12).
		Background(t.Background).
		Border(1, t.Border).
		Radius(controlRadius)
	e.Children(fn)
	return e
}

// tile is one grid cell for an address or prefix. All grids (IPAM, scan and
// IPv6 pages) share this cell so paddings, borders and radii match. A non-nil
// key gives the cell a stable identity before its children are built.
func tile(c *myui.Context, key any, fn func()) myui.Element {
	t := c.Theme()
	e := myui.Column(c).
		Gap(3).
		Padding(7, 9).
		Background(t.Background).
		Border(1, t.Border).
		Radius(controlRadius)
	if key != nil {
		e.Key(key)
	}
	e.Children(fn)
	return e
}

// emptyState is the centered placeholder card for a page with nothing to show.
func emptyState(c *myui.Context, title, description, hint string) {
	t := c.Theme()
	myui.Column(c).Grow(1).Center().Children(func() {
		e := card(c, func() {
			cardHeader(c, title, description, nil)
			if hint != "" {
				myui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
			}
		})
		e.MaxWidth(460)
	})
}

// progressBar shows done/total on a pill-shaped track.
func progressBar(c *myui.Context, done, total float32) {
	t := c.Theme()
	if total <= 0 {
		return
	}
	myui.Row(c).Height(10).Radius(999).Background(t.SurfacePressed).Children(func() {
		if done > 0 {
			myui.Box(c).Height(10).Grow(done).Background(t.Accent).Radius(999)
		}
		if remaining := total - done; remaining > 0 {
			myui.Box(c).Height(10).Grow(remaining).Background(t.SurfacePressed).Radius(999)
		}
	})
}

// infoItem is a small labelled value, shared by the IPv4 and IPv6 results.
func infoItem(c *myui.Context, label, value string) {
	t := c.Theme()
	myui.Column(c).Gap(1).Children(func() {
		myui.Text(c, label).FontSize(10.5).FontWeight(600).
			LetterSpacing(0.8).TextColor(t.TextMuted).SingleLine()
		myui.Text(c, value).FontSize(13).Selectable()
	})
}

// dot is a small state marker.
func dot(c *myui.Context, color myui.Color) myui.Element {
	return myui.Box(c).Width(7).Height(7).Radius(999).Background(color).Shrink(0)
}
