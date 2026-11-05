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

	renaming   bool
	renameName string
	renameErr  string

	confirmDelete bool
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
	s.cancelRename()
	s.confirmDelete = false
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

func (s *ipamState) beginRename() {
	n := s.current()
	if n == nil {
		return
	}
	s.renaming = true
	s.renameName = n.Name
	s.renameErr = ""
}

func (s *ipamState) cancelRename() {
	s.renaming = false
	s.renameName = ""
	s.renameErr = ""
}

func (s *ipamState) rename() {
	if s.store == nil {
		s.renameErr = "The IPAM database is unavailable."
		return
	}
	if s.current() == nil {
		return
	}
	if _, err := s.store.RenameNetwork(s.network, s.renameName); err != nil {
		s.renameErr = "Could not rename the network: " + err.Error()
		return
	}
	s.cancelRename()
	s.refresh()
}

// beginDelete opens the confirmation dialog for the current network.
func (s *ipamState) beginDelete() {
	if s.current() == nil {
		return
	}
	s.confirmDelete = true
}

func (s *ipamState) cancelDelete() {
	s.confirmDelete = false
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
		s.confirmDelete = false // close the dialog so the alert behind it is visible
		s.err = "Could not delete the network: " + err.Error()
		return
	}
	s.confirmDelete = false
	s.network = 0
	s.offset = 0
	s.clearEdit()
	s.cancelRename()
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
	myui.Column(c).Grow(1).Gap(spaceL).Children(func() {
		if s.storeErr != "" {
			alert(c, s.storeErr, t.Danger)
			return
		}
		if s.err != "" {
			alert(c, s.err, t.Danger)
		}

		a.ipamNetworks(c)
		if n := s.current(); n != nil {
			a.ipamNetwork(c, n)
		} else if len(s.networks) == 0 {
			emptyState(c, "No networks saved yet",
				"Calculate an IPv4 block and save it to start the inventory.",
				"Use the IPv4 calculator's Save to IPAM action.")
		}
	})
	a.ipamDeleteDialog(c)
}

// ipamDeleteDialog builds the confirmation dialog shown before a network is
// deleted. It must be built every frame; Modal watches the *bool itself.
func (a *App) ipamDeleteDialog(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Modal(c, &s.confirmDelete, func() {
		n := s.current()
		myui.Column(c).Gap(spaceS).Children(func() {
			myui.Text(c, "Delete network?").FontSize(15).FontWeight(650)
			if n != nil {
				myui.Text(c, fmt.Sprintf(
					"\"%s\" (%s) and its %d assignments will be deleted permanently. This cannot be undone.",
					n.Name, n.CIDR, s.stats.Used,
				)).FontSize(12.5).TextColor(t.TextMuted)
			}
			myui.Row(c).Gap(spaceS).Justify(myui.End).Children(func() {
				myui.Button(c, "Cancel").OnClick(s.cancelDelete)
				myui.Button(c, "Delete").TextColor(t.Danger).OnClick(s.deleteNetwork)
			})
		})
	})
}

func (a *App) ipamNetworks(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	card(c, func() {
		cardHeader(c, "Saved networks",
			"Choose a network to browse and label its usable addresses.", func() {
				pill(c, fmt.Sprintf("%d saved", len(s.networks)), t.Accent)
			})
		if len(s.networks) == 0 {
			myui.Text(c, "No networks saved yet. Calculate an IPv4 block and save it.").
				FontSize(12).TextColor(t.TextMuted)
			return
		}
		myui.Row(c).Wrap().Gap(spaceS).Children(func() {
			for _, n := range s.networks {
				n := n
				myui.Column(c).Key(n.ID).Gap(2).Children(func() {
					var button myui.Element
					if n.ID == s.network {
						button = myui.PrimaryButton(c, n.Name)
					} else {
						button = myui.Button(c, n.Name)
					}
					button.OnClick(func() { s.selectNetwork(n.ID) })
					myui.Text(c, n.CIDR).FontSize(10.5).TextColor(t.TextMuted)
				})
			}
		})
	})
}

func (a *App) ipamNetwork(c *myui.Context, n *ipam.Network) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Grow(1).Gap(spaceL).Children(func() {
		cardHeader(c, n.Name, fmt.Sprintf("%s  ·  %d used, %d free of %d",
			n.CIDR, s.stats.Used, s.stats.Free, s.stats.Total), func() {
			pill(c, fmt.Sprintf("%d used", s.stats.Used), t.Accent)
			pill(c, fmt.Sprintf("%d free", s.stats.Free), t.Success)
			myui.Button(c, "Rename network").OnClick(s.beginRename)
			myui.Button(c, "Delete network").TextColor(t.Danger).OnClick(s.beginDelete)
		})

		if s.renaming {
			inset(c, func() {
				myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
					input := myui.TextInput(c, &s.renameName).
						Label("Network name").
						Placeholder("Office").
						Width(230)
					input.OnSubmit(s.rename)
					myui.PrimaryButton(c, "Save name").OnClick(s.rename)
					myui.Button(c, "Cancel").OnClick(s.cancelRename)
				})
				if s.renameErr != "" {
					alert(c, s.renameErr, t.Danger)
				}
			})
		}

		a.ipamEditor(c)
		a.ipamPaging(c)
		a.ipamGrid(c)
	})
}

func (a *App) ipamEditor(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	inset(c, func() {
		if !s.editAddr.IsValid() {
			myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Children(func() {
				dot(c, t.TextMuted)
				myui.Text(c, "Select an address to label it or mark it used.").
					FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
			myui.Text(c, "Edit "+s.editAddr.String()).FontSize(13).FontWeight(650)
			pill(c, "assignment", t.Accent)
		})
		myui.Row(c).Key(s.editAddr.String()).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
			myui.TextInput(c, &s.editLabel).
				Label("Label").
				Placeholder("Printer, gateway...").
				Width(210)
			myui.TextInput(c, &s.editOwner).
				Label("Owner").
				Placeholder("Team or person").
				Width(190)
			myui.PrimaryButton(c, "Save assignment").OnClick(s.assign)
			myui.Button(c, "Remove assignment").OnClick(s.unassign)
		})
		if s.editErr != "" {
			alert(c, s.editErr, t.Danger)
		}
	})
}

func (a *App) ipamPaging(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
		previous := myui.Button(c, "Previous").OnClick(s.previousPage)
		previous.Disabled(s.offset == 0)
		myui.Text(c, fmt.Sprintf(
			"Page %d of %d - addresses %d-%d of %d",
			s.offset/ipamPageSize+1,
			pageCount(s.stats.Total),
			s.offset+1,
			s.offset+uint64(len(s.entries)),
			s.stats.Total,
		)).FontSize(12).TextColor(t.TextMuted)
		next := myui.Button(c, "Next").OnClick(s.nextPage)
		next.Disabled(s.offset+ipamPageSize >= s.stats.Total)
	})
}

func (a *App) ipamGrid(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Scroll(c).Grow(1).Key(s.network).Children(func() {
		myui.Grid(c).Columns(columnsFor(c, gridCellWidth, 5)).Gap(spaceS).Children(func() {
			for _, entry := range s.entries {
				entry := entry
				color := t.Text
				dotColor := t.Border
				if entry.State == ipam.Used {
					color = t.Accent
					dotColor = t.Accent
				}
				cell := tile(c, nil, func() {
					myui.Row(c).Gap(6).AlignItems(myui.Center).Children(func() {
						dot(c, dotColor)
						myui.Text(c, entry.Addr.String()).FontSize(11.5).TextColor(color)
					})
					myui.Text(c, addressLabel(entry)).FontSize(10).SingleLine().
						TextColor(t.TextMuted)
				})
				if entry.State == ipam.Used {
					cell.Background(t.Accent.Alpha(0.08)).
						BorderColor(t.Accent.Alpha(0.28))
				}
				cell.OnClick(func() { s.beginEdit(entry) })
			}
		})
	})
}

func addressLabel(entry ipam.AddressEntry) string {
	if entry.State != ipam.Used {
		return "free"
	}
	switch {
	case entry.Label != "":
		return entry.Label
	case entry.Owner != "":
		return entry.Owner
	default:
		return "assigned"
	}
}

func pageCount(total uint64) uint64 {
	if total == 0 {
		return 1
	}
	return (total + ipamPageSize - 1) / ipamPageSize
}
