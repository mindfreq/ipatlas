package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/netcalc"
)

const ipv6PageSize = 256

type ipv6State struct {
	address    string
	info       *netcalc.IPv6Info
	addressErr string

	parent    string
	childBits string
	split     *netcalc.IPv6Split
	page      netcalc.IPv6Page
	splitErr  string
}

func (s *ipv6State) inspect() {
	s.info = nil
	s.addressErr = ""

	input := strings.TrimSpace(s.address)
	if input == "" {
		s.addressErr = "Enter an IPv6 address, such as 2001:db8::1."
		return
	}
	info, err := netcalc.ParseIPv6Address(input)
	if err != nil {
		if errors.Is(err, netcalc.ErrNotIPv6) {
			s.addressErr = "That is not an IPv6 address."
		} else {
			s.addressErr = "That address is not valid IPv6."
		}
		return
	}
	s.info = &info
}

func (s *ipv6State) loadPage(offset uint64) {
	s.split = nil
	s.page = netcalc.IPv6Page{}
	s.splitErr = ""

	parent := strings.TrimSpace(s.parent)
	if parent == "" {
		s.splitErr = "Enter a prefix, such as 2001:db8::/48."
		return
	}
	bitsText := strings.TrimSpace(s.childBits)
	if bitsText == "" {
		s.splitErr = "Enter a new prefix length, such as 64."
		return
	}
	bits, err := strconv.Atoi(bitsText)
	if err != nil {
		s.splitErr = "The new prefix length must be a whole number."
		return
	}

	split, err := netcalc.NewIPv6Split(parent, bits)
	if err != nil {
		switch {
		case errors.Is(err, netcalc.ErrNotIPv6):
			s.splitErr = "The prefix must be IPv6, such as 2001:db8::/48."
		case errors.Is(err, netcalc.ErrInvalidCIDR):
			s.splitErr = "The prefix is not a valid CIDR."
		case errors.Is(err, netcalc.ErrInvalidIPv6Split):
			s.splitErr = "The new prefix length must be between the parent length and 128."
		case errors.Is(err, netcalc.ErrIPv6SplitTooLarge):
			s.splitErr = fmt.Sprintf("That split has too many subnets. Add at most %d bits.", netcalc.MaxIPv6SplitBits)
		default:
			s.splitErr = err.Error()
		}
		return
	}

	page, err := split.Page(offset, ipv6PageSize)
	if err != nil {
		s.splitErr = "That page is out of range."
		return
	}
	s.split = &split
	s.page = page
}

func (s *ipv6State) nextPage() {
	if s.split == nil || !s.page.HasNext() {
		return
	}
	page, err := s.split.Page(s.page.Offset+ipv6PageSize, ipv6PageSize)
	if err != nil {
		s.splitErr = "That page is out of range."
		return
	}
	s.splitErr = ""
	s.page = page
}

func (s *ipv6State) previousPage() {
	if s.split == nil || !s.page.HasPrev() {
		return
	}
	offset := uint64(0)
	if s.page.Offset > ipv6PageSize {
		offset = s.page.Offset - ipv6PageSize
	}
	page, err := s.split.Page(offset, ipv6PageSize)
	if err != nil {
		s.splitErr = "That page is out of range."
		return
	}
	s.splitErr = ""
	s.page = page
}

func (a *App) ipv6View(c *myui.Context) {
	t := c.Theme()
	myui.Column(c).Fill().Gap(14).Children(func() {
		myui.Text(c, "IPv6 tools").FontSize(20).Bold()
		myui.Text(c, "Inspect an address, then split a prefix page by page.").TextColor(t.TextMuted)
		a.ipv6Address(c)
		a.ipv6Split(c)
	})
}

func (a *App) ipv6Address(c *myui.Context) {
	s := &a.ipv6
	t := c.Theme()
	myui.Column(c).Gap(8).Children(func() {
		myui.Text(c, "Address").FontSize(16).Bold()
		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.address).Placeholder("2001:db8::1").Label("IPv6 address").Width(360)
			myui.PrimaryButton(c, "Inspect").OnClick(s.inspect)
		})
		if s.addressErr != "" {
			myui.Text(c, s.addressErr).TextColor(t.Danger).Selectable()
		}
		if s.info != nil {
			myui.Column(c).Gap(6).Padding(16).Background(t.Surface).Border(1, t.Border).Radius(8).Children(func() {
				resultRow(c, "Compressed", s.info.Compressed)
				resultRow(c, "Expanded", s.info.Expanded)
				resultRow(c, "Type", s.info.Kind.String())
			})
		}
	})
}

func (a *App) ipv6Split(c *myui.Context) {
	s := &a.ipv6
	t := c.Theme()
	myui.Column(c).Grow(1).Gap(8).Children(func() {
		myui.Text(c, "Split a prefix").FontSize(16).Bold()
		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.parent).Placeholder("2001:db8::/48").Label("IPv6 prefix").Width(320)
			myui.TextInput(c, &s.childBits).Placeholder("64").Label("New prefix length").Width(120)
			myui.PrimaryButton(c, "Split").OnClick(func() {
				s.loadPage(0)
			})
		})
		if s.splitErr != "" {
			myui.Text(c, s.splitErr).TextColor(t.Danger).Selectable()
		}
		if s.split != nil {
			a.ipv6Page(c)
		}
	})
}

func (a *App) ipv6Page(c *myui.Context) {
	s := &a.ipv6
	t := c.Theme()
	myui.Column(c).Grow(1).Gap(8).Children(func() {
		myui.Text(c, fmt.Sprintf(
			"%d /%d subnets in %s",
			s.page.Total, s.split.ChildBits(), s.split.Parent(),
		)).TextColor(t.TextMuted)

		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			previous := myui.Button(c, "Previous").OnClick(s.previousPage)
			previous.Disabled(!s.page.HasPrev())
			myui.Text(c, fmt.Sprintf(
				"Page %d of %d - subnets %d-%d of %d",
				s.page.Offset/ipv6PageSize+1,
				(s.page.Total+ipv6PageSize-1)/ipv6PageSize,
				s.page.Offset+1,
				s.page.End(),
				s.page.Total,
			))
			next := myui.Button(c, "Next").OnClick(s.nextPage)
			next.Disabled(!s.page.HasNext())
		})

		myui.Scroll(c).Grow(1).Key(s.page.Offset).Children(func() {
			myui.Grid(c).Columns(4).Gap(6).Children(func() {
				for _, prefix := range s.page.Prefixes {
					myui.Text(c, prefix.String()).SingleLine().Selectable()
				}
			})
		})
	})
}
