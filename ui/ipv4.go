package ui

import (
	"errors"
	"strconv"
	"strings"

	myui "github.com/egoist/mygo/ui"

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
	myui.Column(c).Fill().Gap(14).Children(func() {
		myui.Text(c, "IPv4 calculator").FontSize(20).Bold()
		myui.Text(c, "Enter an IPv4 CIDR block to calculate its address plan.").TextColor(t.TextMuted)
		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.input).Placeholder("192.168.1.0/24").Label("CIDR").Width(280)
			myui.PrimaryButton(c, "Calculate").OnClick(s.calculate)
		})
		if s.err != "" {
			myui.Text(c, s.err).TextColor(t.Danger).Selectable()
		}
		if s.result != nil {
			a.ipv4Result(c, s.result)
		}
	})
}

func (a *App) ipv4Result(c *myui.Context, r *netcalc.IPv4Subnet) {
	s := &a.ipv4
	t := c.Theme()
	myui.Column(c).Gap(6).Padding(16).Background(t.Surface).Border(1, t.Border).Radius(8).Children(func() {
		myui.Text(c, cidrString(*r)).FontSize(18).Bold().Selectable()
		if r.Address != r.Network {
			resultRow(c, "Entered address", r.Address.String())
		}
		resultRow(c, "Broadcast", r.Broadcast.String())
		resultRow(c, "Netmask", r.Mask.String())
		resultRow(c, "Wildcard", r.Wildcard.String())
		resultRow(c, "Usable range", r.First.String()+" - "+r.Last.String())
		resultRow(c, "Usable hosts", strconv.FormatUint(r.Usable, 10))
		resultRow(c, "Gateway", r.Gateway.String())
		myui.Divider(c)
		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.saveName).Label("Network name").Placeholder("Office").Width(220)
			myui.PrimaryButton(c, "Save to IPAM").OnClick(a.saveCurrentIPv4)
		})
		if s.saveMsg != "" {
			color := t.Success
			if s.saveErr {
				color = t.Danger
			}
			myui.Text(c, s.saveMsg).TextColor(color).Selectable()
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

	saved, err := a.ipam.saveNetwork(cidrString(*s.result), s.saveName)
	if err != nil {
		s.saveErr = true
		s.saveMsg = "Could not save to IPAM: " + err.Error()
		return
	}
	s.saveName = ""
	s.saveMsg = "Saved " + saved.CIDR + " to IPAM."
}

func resultRow(c *myui.Context, label, value string) {
	t := c.Theme()
	myui.Row(c).Gap(12).Children(func() {
		myui.Text(c, label).Width(140).TextColor(t.TextMuted)
		myui.Text(c, value).Selectable()
	})
}

func cidrString(s netcalc.IPv4Subnet) string {
	return s.Network.String() + "/" + strconv.Itoa(s.Prefix)
}
