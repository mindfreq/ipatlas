package ui

import (
	myui "github.com/egoist/mygo/ui"
)

// repoURL is the project's public source repository.
const repoURL = "https://github.com/mindfreq/ipatlas"

// aboutView shows the app's identity, how it treats data, and where to find
// its source and license. The page has no editable state, so it has none of
// its own either.
func (a *App) aboutView(c *myui.Context) {
	t := c.Theme()
	myui.Scroll(c).Grow(1).Children(func() {
		myui.Column(c).Gap(spaceL).Children(func() {
			a.aboutHero(c, t)
			card(c, func() {
				cardHeader(c, "Your data stays here", "IPAtlas has no account, no sync and no telemetry.", nil)
				myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Children(func() {
					dot(c, t.Success)
					myui.Text(c, "Everything you enter is held in this process's memory or on this machine's disk.").FontSize(12.5)
				})
				myui.Text(c, "Saved networks and host labels live in ipatlas.db inside your user data directory. The scanner only sends probes to the network you type into it and never phone home.").FontSize(12.5).
					TextColor(t.TextMuted)
			})
			card(c, func() {
				cardHeader(c, "Source and license", "Open source, inspected and rebuilt with confidence.", nil)
				myui.Link(c, repoURL, repoURL)
				myui.Text(c, "IPAtlas is released under the MIT License: free to run, study, modify and share, with the copyright notice preserved.").FontSize(12.5).
					TextColor(t.TextMuted)
			})
		})
	})
}

// aboutHero is the identity card: logo, name, tagline and the version pill.
func (a *App) aboutHero(c *myui.Context, t *myui.Theme) {
	card(c, func() {
		myui.Row(c).Gap(spaceM).AlignItems(myui.Center).Wrap().Children(func() {
			a.logoMark(c, t)
			myui.Column(c).Gap(1).MinWidth(0).Children(func() {
				myui.Text(c, "IPAtlas").FontSize(19).FontWeight(700)
				myui.Text(c, "Subnet toolkit for the desktop").FontSize(12).TextColor(t.TextMuted)
			})
			if a.version == "" {
				pill(c, "Development build", t.Accent)
			} else {
				pill(c, "Version "+a.version, t.Accent)
			}
		})
		myui.Text(c, "IPAtlas gathers an IPv4 calculator, a VLSM planner, IPv6 prefix tools, a small IP address manager and a bounded network scanner into one offline app. Every result you see is computed locally, and each tool can hand its output to the next, from a calculated block to a saved, labelled and scanned network.").FontSize(13)
	})
}
