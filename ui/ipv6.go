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
	myui.Scroll(c).Grow(1).Children(func() {
		myui.Column(c).Gap(spaceL).Children(func() {
			myui.Row(c).Wrap().Gap(spaceL).AlignItems(myui.Stretch).Children(func() {
				a.ipv6Address(c)
				a.ipv6Split(c)
			})
			if a.ipv6.split != nil {
				a.ipv6Page(c)
			} else if a.ipv6.splitErr == "" && a.ipv6.info == nil && a.ipv6.addressErr == "" {
				myui.Text(c, "Inspect an address or split a prefix to begin.").
					FontSize(12).TextColor(t.TextMuted)
			}
		})
	})
}

func (a *App) ipv6Address(c *myui.Context) {
	s := &a.ipv6
	t := c.Theme()
	address := card(c, func() {
		cardHeader(c, "Address inspector", "Compress, expand and classify any IPv6 address.", nil)
		myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Children(func() {
			input := myui.TextInput(c, &s.address).
				Placeholder("2001:db8::1").
				Label("IPv6 address").
				Grow(1).
				MinWidth(160)
			input.OnSubmit(s.inspect)
			myui.PrimaryButton(c, "Inspect").OnClick(s.inspect)
		})
		if s.addressErr != "" {
			alert(c, s.addressErr, t.Danger)
		}
		if s.info != nil {
			myui.Divider(c)
			myui.Column(c).Gap(spaceS).Children(func() {
				infoItem(c, "Compressed", s.info.Compressed)
				infoItem(c, "Expanded", s.info.Expanded)
				infoItem(c, "Type", s.info.Kind.String())
			})
		}
	})
	address.Grow(1).MinWidth(330)
}

func (a *App) ipv6Split(c *myui.Context) {
	s := &a.ipv6
	t := c.Theme()
	split := card(c, func() {
		cardHeader(c, "Split a prefix", "Page through child prefixes without listing them all.", nil)
		myui.Row(c).Children(func() {
			parent := myui.TextInput(c, &s.parent).
				Placeholder("2001:db8::/48").
				Label("IPv6 prefix").
				Grow(1).
				MinWidth(160)
			parent.OnSubmit(func() { s.loadPage(0) })
		})
		myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Children(func() {
			bits := myui.TextInput(c, &s.childBits).
				Placeholder("64").
				Label("New prefix length").
				Width(120)
			bits.OnSubmit(func() { s.loadPage(0) })
			myui.PrimaryButton(c, "Split").OnClick(func() { s.loadPage(0) })
			myui.Text(c, "new prefix bits").FontSize(11.5).TextColor(t.TextMuted)
		})
		if s.splitErr != "" {
			alert(c, s.splitErr, t.Danger)
		}
	})
	split.Grow(1).MinWidth(330)
}

func (a *App) ipv6Page(c *myui.Context) {
	s := &a.ipv6
	card(c, func() {
		cardHeader(c, fmt.Sprintf(
			"%d /%d subnets in %s",
			s.page.Total, s.split.ChildBits(), s.split.Parent(),
		), fmt.Sprintf(
			"Page %d of %d - subnets %d-%d of %d",
			s.page.Offset/ipv6PageSize+1,
			(s.page.Total+ipv6PageSize-1)/ipv6PageSize,
			s.page.Offset+1,
			s.page.End(),
			s.page.Total,
		), func() {
			previous := myui.Button(c, "Previous").OnClick(s.previousPage)
			previous.Disabled(!s.page.HasPrev())
			next := myui.Button(c, "Next").OnClick(s.nextPage)
			next.Disabled(!s.page.HasNext())
		})

		myui.Scroll(c).Height(220).Key(s.page.Offset).Children(func() {
			myui.Grid(c).Columns(columnsFor(c, 160, 4)).Gap(spaceS).Children(func() {
				for _, prefix := range s.page.Prefixes {
					tile(c, nil, func() {
						myui.Text(c, prefix.String()).FontSize(11.5).SingleLine().Selectable()
					})
				}
			})
		})
	})
}
