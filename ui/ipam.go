package ui

import (
	"errors"
	"fmt"
	"net/netip"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
)

const ipamPageSize = 256

type ipamState struct {
	store    *ipam.Store
	storeErr string

	networks []ipam.Network
	network  int64

	entries []ipam.AddressEntry
	stats   ipam.Stats
	offset  uint64
	err     string

	editAddr  netip.Addr
	editLabel string
	editOwner string
	editErr   string
}

func (s *ipamState) attach(store *ipam.Store) {
	s.store = store
	s.err = ""
	s.storeErr = ""
	if store == nil {
		s.storeErr = "The IPAM database is unavailable."
		return
	}
	s.refresh()
}

// refresh reloads the saved networks and keeps the selected one when it still
// exists.
func (s *ipamState) refresh() {
	s.err = ""
	if s.store == nil {
		s.networks = nil
		s.entries = nil
		s.network = 0
		return
	}

	networks, err := s.store.Networks()
	if err != nil {
		s.err = "Could not read saved networks: " + err.Error()
		return
	}
	s.networks = networks

	if len(networks) == 0 {
		s.network = 0
		s.entries = nil
		s.stats = ipam.Stats{}
		s.clearEdit()
		return
	}

	found := false
	for _, n := range networks {
		if n.ID == s.network {
			found = true
			break
		}
	}
	if !found {
		s.network = networks[0].ID
		s.offset = 0
		s.clearEdit()
	}
	s.loadPage()
}

func (s *ipamState) current() *ipam.Network {
	for i := range s.networks {
		if s.networks[i].ID == s.network {
			return &s.networks[i]
		}
	}
	return nil
}

func (s *ipamState) selectNetwork(id int64) {
	if s.network == id {
		return
	}
	s.network = id
	s.offset = 0
	s.clearEdit()
	s.loadPage()
}

func (s *ipamState) loadPage() {
	s.err = ""
	s.editErr = ""
	if s.store == nil {
		s.storeErr = "The IPAM database is unavailable."
		return
	}
	if s.current() == nil {
		s.entries = nil
		s.stats = ipam.Stats{}
		return
	}

	entries, err := s.store.Addresses(s.network, s.offset, ipamPageSize)
	if err != nil {
		if errors.Is(err, ipam.ErrOffsetOutOfRange) {
			s.offset = 0
			entries, err = s.store.Addresses(s.network, 0, ipamPageSize)
		}
		if err != nil {
			s.err = "Could not read addresses: " + err.Error()
			return
		}
	}
	stats, err := s.store.Stats(s.network)
	if err != nil {
		s.err = "Could not read address counts: " + err.Error()
		return
	}
	s.entries = entries
	s.stats = stats
}

func (s *ipamState) nextPage() {
	if s.store == nil || s.stats.Total == 0 {
		return
	}
	next := s.offset + ipamPageSize
	if next >= s.stats.Total {
		return
	}
	s.offset = next
	s.loadPage()
}

func (s *ipamState) previousPage() {
	if s.store == nil || s.offset == 0 {
		return
	}
	if s.offset > ipamPageSize {
		s.offset -= ipamPageSize
	} else {
		s.offset = 0
	}
	s.loadPage()
}

func (s *ipamState) beginEdit(entry ipam.AddressEntry) {
	s.editAddr = entry.Addr
	s.editLabel = entry.Label
	s.editOwner = entry.Owner
	s.editErr = ""
}

func (s *ipamState) clearEdit() {
	s.editAddr = netip.Addr{}
	s.editLabel = ""
	s.editOwner = ""
	s.editErr = ""
}

func (s *ipamState) assign() {
	if s.store == nil {
		s.editErr = "The IPAM database is unavailable."
		return
	}
	if !s.editAddr.IsValid() {
		s.editErr = "Select an address first."
		return
	}
	if err := s.store.Assign(s.network, s.editAddr, s.editLabel, s.editOwner); err != nil {
		s.editErr = "Could not save the assignment: " + err.Error()
		return
	}
	s.loadPage()
}

func (s *ipamState) unassign() {
	if s.store == nil {
		s.editErr = "The IPAM database is unavailable."
		return
	}
	if !s.editAddr.IsValid() {
		s.editErr = "Select an address first."
		return
	}
	if err := s.store.Unassign(s.network, s.editAddr); err != nil {
		s.editErr = "Could not remove the assignment: " + err.Error()
		return
	}
	s.loadPage()
}

func (s *ipamState) deleteNetwork() {
	if s.store == nil {
		s.err = "The IPAM database is unavailable."
		return
	}
	if s.network == 0 {
		return
	}
	if err := s.store.DeleteNetwork(s.network); err != nil {
		s.err = "Could not delete the network: " + err.Error()
		return
	}
	s.network = 0
	s.offset = 0
	s.clearEdit()
	s.refresh()
}

func (s *ipamState) saveNetwork(cidr, name string) (ipam.Network, error) {
	if s.store == nil {
		return ipam.Network{}, fmt.Errorf("the IPAM database is unavailable")
	}
	saved, err := s.store.SaveNetwork(cidr, name)
	if err != nil {
		return ipam.Network{}, err
	}
	s.network = saved.ID
	s.offset = 0
	s.refresh()
	return saved, nil
}

func (a *App) ipamView(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Fill().Gap(12).Children(func() {
		myui.Text(c, "IP address management").FontSize(20).Bold()
		myui.Text(c, "Saved networks and their labelled addresses, stored locally.").TextColor(t.TextMuted)

		if s.storeErr != "" {
			myui.Text(c, s.storeErr).TextColor(t.Danger).Selectable()
			return
		}
		if s.err != "" {
			myui.Text(c, s.err).TextColor(t.Danger).Selectable()
		}

		a.ipamNetworks(c)
		if n := s.current(); n != nil {
			a.ipamNetwork(c, n)
		} else if len(s.networks) == 0 {
			myui.Text(c, "No networks saved yet. Calculate an IPv4 block and save it.").TextColor(t.TextMuted)
		}
	})
}

func (a *App) ipamNetworks(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Gap(6).Children(func() {
		myui.Text(c, "Saved networks").FontSize(16).Bold()
		if len(s.networks) == 0 {
			return
		}
		myui.Grid(c).Columns(3).Gap(6).Children(func() {
			for _, n := range s.networks {
				n := n
				button := myui.Button(c, n.Name)
				if n.ID == s.network {
					button = myui.PrimaryButton(c, n.Name)
				}
				button.OnClick(func() { s.selectNetwork(n.ID) })
				myui.Text(c, n.CIDR).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}

func (a *App) ipamNetwork(c *myui.Context, n *ipam.Network) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Grow(1).Gap(10).Children(func() {
		myui.Row(c).Gap(12).AlignItems(myui.Center).Children(func() {
			myui.Text(c, n.Name).FontSize(18).Bold()
			myui.Text(c, n.CIDR).TextColor(t.TextMuted)
			myui.Text(c, fmt.Sprintf("%d used, %d free of %d", s.stats.Used, s.stats.Free, s.stats.Total)).
				TextColor(t.TextMuted)
			myui.Button(c, "Delete network").OnClick(s.deleteNetwork)
		})

		a.ipamEditor(c)
		a.ipamPaging(c)
		a.ipamGrid(c)
	})
}

func (a *App) ipamGrid(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Scroll(c).Grow(1).Key(s.network).Children(func() {
		myui.Grid(c).Columns(4).Gap(4).Children(func() {
			for _, entry := range s.entries {
				entry := entry
				myui.Column(c).Key(entry.Addr.String()).Gap(2).Children(func() {
					button := myui.Button(c, entry.Addr.String())
					if entry.State == ipam.Used {
						button.TextColor(t.Accent)
					} else {
						button.TextColor(t.TextMuted)
					}
					button.OnClick(func() { s.beginEdit(entry) })
					if entry.State == ipam.Used {
						label := entry.Label
						if label == "" {
							label = entry.Owner
						}
						if label == "" {
							label = "assigned"
						}
						myui.Text(c, label).FontSize(11).SingleLine().TextColor(t.TextMuted)
					} else {
						myui.Text(c, "free").FontSize(11).TextColor(t.TextMuted)
					}
				})
			}
		})
	})
}

func (a *App) ipamEditor(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Gap(8).Padding(12).Background(t.Surface).Border(1, t.Border).Radius(8).Children(func() {
		if !s.editAddr.IsValid() {
			myui.Text(c, "Select an address to label it or mark it used.").TextColor(t.TextMuted)
			return
		}
		myui.Text(c, "Edit "+s.editAddr.String()).Bold()
		myui.Row(c).Key(s.editAddr.String()).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.editLabel).Label("Label").Placeholder("Printer, gateway...").Width(200)
			myui.TextInput(c, &s.editOwner).Label("Owner").Placeholder("Team or person").Width(180)
			myui.PrimaryButton(c, "Save assignment").OnClick(s.assign)
			myui.Button(c, "Remove assignment").OnClick(s.unassign)
		})
		if s.editErr != "" {
			myui.Text(c, s.editErr).TextColor(t.Danger).Selectable()
		}
	})
}

func (a *App) ipamPaging(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
		previous := myui.Button(c, "Previous").OnClick(s.previousPage)
		previous.Disabled(s.offset == 0)
		myui.Text(c, fmt.Sprintf(
			"Page %d of %d - addresses %d-%d of %d",
			s.offset/ipamPageSize+1,
			pageCount(s.stats.Total),
			s.offset+1,
			s.offset+uint64(len(s.entries)),
			s.stats.Total,
		)).TextColor(t.TextMuted)
		next := myui.Button(c, "Next").OnClick(s.nextPage)
		next.Disabled(s.offset+ipamPageSize >= s.stats.Total)
	})
}

func pageCount(total uint64) uint64 {
	if total == 0 {
		return 1
	}
	return (total + ipamPageSize - 1) / ipamPageSize
}
