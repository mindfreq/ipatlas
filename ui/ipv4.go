package ui

import (
	"errors"
	"strconv"
	"strings"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
	"ipatlas/internal/netcalc"
)

type ipv4State struct {
	input    string
	result   *netcalc.IPv4Subnet
	err      string
	saveName string
	saveMsg  string
	saveErr  bool
}

func (s *ipv4State) calculate() {
	s.result = nil
	s.err = ""
	s.saveMsg = ""
	s.saveErr = false

	input := strings.TrimSpace(s.input)
	if input == "" {
		s.err = "Enter a CIDR block, such as 192.168.1.0/24."
		return
	}

	result, err := netcalc.ParseIPv4CIDR(input)
	if err != nil {
		if errors.Is(err, netcalc.ErrNotIPv4) {
			s.err = "That is not an IPv4 CIDR. Try 192.168.1.0/24."
		} else {
			s.err = "That CIDR is not valid. Try 192.168.1.0/24."
		}
		return
	}
	s.result = &result
}

func (a *App) ipv4View(c *myui.Context) {
	s := &a.ipv4
	t := c.Theme()
	myui.Scroll(c).Grow(1).Children(func() {
		myui.Column(c).Gap(spaceL).Children(func() {
			card(c, func() {
				cardHeader(c, "Calculate a block",
					"Any IPv4 CIDR from /0 to /32; host bits are normalized.", nil)
				myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
					input := myui.TextInput(c, &s.input).
						Placeholder("192.168.1.0/24").
						Label("CIDR").
						Width(300)
					input.OnSubmit(s.calculate)
					myui.PrimaryButton(c, "Calculate").OnClick(s.calculate)
				})
				if s.err != "" {
					alert(c, s.err, t.Danger)
				}
			})
			if s.result != nil {
				a.ipv4Result(c, s.result)
			}
		})
	})
}

func (a *App) ipv4Result(c *myui.Context, r *netcalc.IPv4Subnet) {
	s := &a.ipv4
	t := c.Theme()
	card(c, func() {
		cardHeader(c, cidrString(*r), "Address plan", func() {
			pill(c, "IPv4", t.Accent)
		})

		myui.Divider(c)

		myui.Grid(c).Columns(2).Gap(spaceM).Children(func() {
			infoItem(c, "Network", r.Network.String())
			infoItem(c, "Broadcast", r.Broadcast.String())
			infoItem(c, "Netmask", r.Mask.String())
			infoItem(c, "Wildcard", r.Wildcard.String())
			infoItem(c, "Usable range", r.First.String()+" - "+r.Last.String())
			infoItem(c, "Usable hosts", strconv.FormatUint(r.Usable, 10))
			infoItem(c, "Gateway", r.Gateway.String())
			if r.Address != r.Network {
				infoItem(c, "Entered address", r.Address.String())
			}
			infoItem(c, "Prefix", "/"+strconv.Itoa(r.Prefix))
		})

		myui.Divider(c)
		sectionLabel(c, "ADD TO INVENTORY")
		myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
			myui.TextInput(c, &s.saveName).
				Label("Network name").
				Placeholder("Office").
				Width(230)
			myui.PrimaryButton(c, "Save to IPAM").OnClick(a.saveCurrentIPv4)
			myui.Text(c, "Stored in the local IPAM database.").FontSize(11.5).TextColor(t.TextMuted)
		})
		if s.saveMsg != "" {
			color := t.Success
			if s.saveErr {
				color = t.Danger
			}
			alert(c, s.saveMsg, color)
		}
	})
}

// saveCurrentIPv4 saves the calculated block in the IPAM database.
func (a *App) saveCurrentIPv4() {
	s := &a.ipv4
	s.saveMsg = ""
	s.saveErr = false
	if s.result == nil {
		return
	}

	// The save runs off the UI thread; the callback reports back on it.
	a.ipam.saveNetwork(cidrString(*s.result), s.saveName, func(saved ipam.Network, err error) {
		if err != nil {
			s.saveErr = true
			s.saveMsg = "Could not save to IPAM: " + err.Error()
			return
		}
		s.saveName = ""
		s.saveMsg = "Saved " + saved.CIDR + " to IPAM."
	})
}

func cidrString(s netcalc.IPv4Subnet) string {
	return s.Network.String() + "/" + strconv.Itoa(s.Prefix)
}
