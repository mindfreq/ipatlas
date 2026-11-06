package ui

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
)

const ipamPageSize = 256

const (
	// With more saved networks than this, the list scrolls inside a fixed
	// height instead of growing and pushing the address grid off screen.
	networksScrollThreshold = 6
	networksListHeight      = 120

	// Below these window sizes the page switches to a tighter layout.
	ipamCompactHeight = 720
	ipamNarrowWidth   = 1000
)

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

	// View options.
	networksHidden bool // user choice for the "Saved networks" list
	networksSet    bool // the user made that choice; otherwise it follows the window height
	addressesFocus bool // only the address grid and its controls are shown

	// Search. While searching, the grid shows the matching assignments
	// instead of a page of the address range.
	searchInput  string // text in the search box
	searchQuery  string // the text query that produced results
	searching    bool
	searchOffset uint64 // index among the matches, not an address offset
	results      []ipam.AddressEntry
	matches      uint64
	searchMsg    string

	// reveal asks the grid to scroll the selected address into view once.
	reveal bool

	// Async plumbing. Database work runs on a goroutine and reports back on
	// the UI thread, so a slow database never blocks a frame. gen drops
	// results of superseded jobs; busy disables the buttons that load.
	gen        uint64
	busy       bool
	invalidate func() // the window's Invalidate; set on every frame
}

// runOffUI runs job on another goroutine and then fn on the UI thread. It is
// a variable so tests can run the whole round-trip synchronously and keep
// the state deterministic.
var runOffUI = func(job, fn func()) {
	go func() {
		job()
		mygo.RunOnMain(fn)
	}()
}

// schedule runs job off the UI thread and fn back on it. fn is skipped when
// another job started in the meantime; its result is stale. Both job's
// writes and fn must only touch state through the closure variables job
// filled and fn reads: state fields are only touched on the UI thread.
func (s *ipamState) schedule(job, fn func()) {
	s.gen++
	gen := s.gen
	s.busy = true
	runOffUI(job, func() {
		if gen != s.gen {
			return
		}
		s.busy = false
		fn()
		s.redraw()
	})
}

// redraw asks the window for a new frame; nothing to ask for before the
// first frame, which reads the state as it is then.
func (s *ipamState) redraw() {
	if s.invalidate != nil {
		s.invalidate()
	}
}

// pageResult is what one page load gathers off the UI thread.
type pageResult struct {
	searching    bool
	stats        ipam.Stats
	entries      []ipam.AddressEntry
	results      []ipam.AddressEntry
	matches      uint64
	offset       uint64 // the offset the entries were read at, after fixes
	searchOffset uint64
	err          string
}

// fetchPage gathers the counts and the entries the grid shows for the given
// selection: the matches while searching, else a page of the address range.
// It runs on the job goroutine, so it must not touch state.
func fetchPage(store *ipam.Store, network int64, offset uint64, searching bool, query string, searchOffset uint64) pageResult {
	var res pageResult
	res.searching = searching
	stats, err := store.Stats(network)
	if err != nil {
		res.err = "Could not read address counts: " + err.Error()
		return res
	}
	res.stats = stats

	if searching {
		results, total, err := store.SearchAssignments(network, query, searchOffset, ipamPageSize)
		if err == nil && len(results) == 0 && total > 0 && searchOffset > 0 {
			// The page vanished, e.g. after removing assignments: start over.
			searchOffset = 0
			results, total, err = store.SearchAssignments(network, query, 0, ipamPageSize)
		}
		if err != nil {
			res.err = "Could not search: " + err.Error()
			return res
		}
		res.results = results
		res.matches = total
		res.searchOffset = searchOffset
		return res
	}

	entries, err := store.Addresses(network, offset, ipamPageSize)
	if err != nil {
		if errors.Is(err, ipam.ErrOffsetOutOfRange) {
			offset = 0
			entries, err = store.Addresses(network, 0, ipamPageSize)
		}
		if err != nil {
			res.err = "Could not read addresses: " + err.Error()
			return res
		}
	}
	res.entries = entries
	res.offset = offset
	return res
}

// applyPage stores what a finished page load delivered.
func (s *ipamState) applyPage(p pageResult) {
	if p.err != "" {
		s.err = p.err
		return
	}
	s.stats = p.stats
	if p.searching {
		s.results = p.results
		s.matches = p.matches
		s.searchOffset = p.searchOffset
	} else {
		s.entries = p.entries
		s.offset = p.offset
	}
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
// exists. The database work runs off the UI thread.
func (s *ipamState) refresh() {
	s.err = ""
	if s.store == nil {
		s.networks = nil
		s.entries = nil
		s.network = 0
		return
	}

	store := s.store
	keep, keepOffset := s.network, s.offset
	var (
		networks     []ipam.Network
		loadErr      string
		selected     int64
		resetToFirst bool
		page         pageResult
	)
	s.schedule(func() {
		ns, err := store.Networks()
		if err != nil {
			loadErr = "Could not read saved networks: " + err.Error()
			return
		}
		networks = ns
		if len(ns) == 0 {
			return
		}
		selected = keep
		for i := range ns {
			if ns[i].ID == keep {
				page = fetchPage(store, selected, keepOffset, false, "", 0)
				return
			}
		}
		selected = ns[0].ID
		resetToFirst = true
		page = fetchPage(store, selected, 0, false, "", 0)
	}, func() {
		if loadErr != "" {
			s.err = loadErr
			return
		}
		s.networks = networks
		if len(networks) == 0 {
			s.network = 0
			s.entries = nil
			s.stats = ipam.Stats{}
			s.resetSearch()
			s.clearEdit()
			return
		}
		s.network = selected
		if resetToFirst {
			s.offset = 0
			s.resetSearch()
			s.clearEdit()
		}
		s.applyPage(page)
	})
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
	s.resetSearch()
	s.clearEdit()
	s.cancelRename()
	s.confirmDelete = false
	s.loadPage()
}

// loadPage reloads the counts and the entries the grid shows: the matches
// while searching, else the page of the address range at offset. The
// database work runs off the UI thread.
func (s *ipamState) loadPage() {
	s.err = ""
	s.editErr = ""
	if s.store == nil {
		s.storeErr = "The IPAM database is unavailable."
		return
	}
	if s.current() == nil {
		s.entries = nil
		s.results = nil
		s.stats = ipam.Stats{}
		return
	}

	store := s.store
	network, offset := s.network, s.offset
	searching, query, searchOffset := s.searching, s.searchQuery, s.searchOffset
	var page pageResult
	s.schedule(func() {
		page = fetchPage(store, network, offset, searching, query, searchOffset)
	}, func() {
		s.applyPage(page)
	})
}

// shown is the list of entries the grid draws.
func (s *ipamState) shown() []ipam.AddressEntry {
	if s.searching {
		return s.results
	}
	return s.entries
}

// pager returns the offset of the current page and the size of the list it
// pages through.
func (s *ipamState) pager() (offset, total uint64) {
	if s.searching {
		return s.searchOffset, s.matches
	}
	return s.offset, s.stats.Total
}

func (s *ipamState) setOffset(v uint64) {
	if s.searching {
		s.searchOffset = v
	} else {
		s.offset = v
	}
	s.loadPage()
}

func (s *ipamState) nextPage() {
	offset, total := s.pager()
	next := offset + ipamPageSize
	if s.store == nil || next >= total {
		return
	}
	s.setOffset(next)
}

func (s *ipamState) previousPage() {
	offset, _ := s.pager()
	if s.store == nil || offset == 0 {
		return
	}
	if offset > ipamPageSize {
		s.setOffset(offset - ipamPageSize)
	} else {
		s.setOffset(0)
	}
}

// pageLabel is the text between the page buttons.
func (s *ipamState) pageLabel(narrow bool) string {
	offset, total := s.pager()
	page := offset/ipamPageSize + 1
	pages := pageCount(total)
	if s.searching {
		switch {
		case total == 0:
			return "No matches"
		case narrow:
			return fmt.Sprintf("%d found · %d/%d", total, page, pages)
		default:
			return fmt.Sprintf("%d matches · page %d of %d", total, page, pages)
		}
	}
	if narrow {
		return fmt.Sprintf("%d / %d", page, pages)
	}
	return fmt.Sprintf("Page %d of %d · %d-%d of %d",
		page, pages, offset+1, offset+uint64(len(s.entries)), total)
}

// networksCollapsed reports whether the "Saved networks" list is folded: the
// user's choice, else folded in short windows to leave room for the grid.
func (s *ipamState) networksCollapsed(compact bool) bool {
	if s.networksSet {
		return s.networksHidden
	}
	return compact
}

func (s *ipamState) toggleNetworks(compact bool) {
	s.networksHidden = !s.networksCollapsed(compact)
	s.networksSet = true
}

// resetSearch leaves search mode without reloading.
func (s *ipamState) resetSearch() {
	s.searching = false
	s.searchInput = ""
	s.searchQuery = ""
	s.searchOffset = 0
	s.results = nil
	s.matches = 0
	s.searchMsg = ""
}

func (s *ipamState) clearSearch() {
	s.resetSearch()
	if s.store != nil && s.current() != nil {
		s.loadPage()
	}
}

// submitSearch runs what is in the search box: a full IPv4 address jumps to
// it, any other text searches the assignments, empty text clears the search.
func (s *ipamState) submitSearch() {
	s.searchMsg = ""
	query := strings.TrimSpace(s.searchInput)
	if query == "" {
		s.clearSearch()
		return
	}
	if s.store == nil || s.current() == nil {
		return
	}
	if addr, err := netip.ParseAddr(query); err == nil && addr.Is4() {
		s.jumpTo(addr)
		return
	}
	s.searching = true
	s.searchQuery = query
	s.searchOffset = 0
	s.loadPage()
}

// jumpTo shows the page that holds addr and selects it. The database work
// runs off the UI thread.
func (s *ipamState) jumpTo(addr netip.Addr) {
	store := s.store
	network := s.network
	var (
		page         pageResult
		offset       uint64
		notInNetwork bool
		loadErr      string
	)
	s.schedule(func() {
		off, err := store.Offset(network, addr)
		if err != nil {
			if errors.Is(err, ipam.ErrAddressNotInNetwork) {
				notInNetwork = true
			} else {
				loadErr = "Could not find the address: " + err.Error()
			}
			return
		}
		offset = off
		page = fetchPage(store, network, off/ipamPageSize*ipamPageSize, false, "", 0)
	}, func() {
		if notInNetwork {
			s.searchMsg = addr.String() + " is not a usable address in this network."
			return
		}
		if loadErr != "" {
			s.err = loadErr
			return
		}
		s.searching = false
		s.searchQuery = ""
		s.searchOffset = 0
		s.results = nil
		s.matches = 0
		s.searchInput = addr.String()
		s.applyPage(page)
		s.selectByOffset(offset)
	})
}

// selectByOffset starts editing the loaded entry at offset, if there is one.
func (s *ipamState) selectByOffset(offset uint64) {
	for _, e := range s.entries {
		if e.Offset == offset {
			s.beginEdit(e)
			s.reveal = true
			return
		}
	}
}

// locateEdit leaves the search results and shows the selected address in its
// place in the range.
func (s *ipamState) locateEdit() {
	if s.store == nil || !s.editAddr.IsValid() {
		return
	}
	s.jumpTo(s.editAddr)
}

// nextFree selects the first free address after the selected one, wrapping to
// the start of the network. Repeated clicks walk through the free addresses.
// The database work runs off the UI thread.
func (s *ipamState) nextFree() {
	s.searchMsg = ""
	if s.store == nil || s.current() == nil {
		return
	}
	store := s.store
	network := s.network
	fromAddr := s.editAddr
	var (
		page    pageResult
		entry   ipam.AddressEntry
		found   bool
		loadErr string
		noFree  bool
	)
	s.schedule(func() {
		var from uint64
		if fromAddr.IsValid() {
			if offset, err := store.Offset(network, fromAddr); err == nil {
				from = offset + 1
			}
		}
		e, ok, err := store.NextFree(network, from)
		if err == nil && !ok && from > 0 {
			e, ok, err = store.NextFree(network, 0)
		}
		if err != nil {
			loadErr = "Could not find a free address: " + err.Error()
			return
		}
		if !ok {
			noFree = true
			return
		}
		entry, found = e, true
		page = fetchPage(store, network, e.Offset/ipamPageSize*ipamPageSize, false, "", 0)
	}, func() {
		if loadErr != "" {
			s.err = loadErr
			return
		}
		if noFree {
			s.searchMsg = "This network has no free addresses."
			return
		}
		s.searching = false
		s.searchQuery = ""
		s.searchOffset = 0
		s.results = nil
		s.matches = 0
		s.searchInput = ""
		s.applyPage(page)
		if found {
			s.beginEdit(entry)
			s.reveal = true
		}
	})
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

// assign saves the edited assignment and reloads the page. The database
// work runs off the UI thread.
func (s *ipamState) assign() {
	if s.store == nil {
		s.editErr = "The IPAM database is unavailable."
		return
	}
	if !s.editAddr.IsValid() {
		s.editErr = "Select an address first."
		return
	}
	store := s.store
	network, addr, label, owner := s.network, s.editAddr, s.editLabel, s.editOwner
	offset, searching, query, searchOffset := s.offset, s.searching, s.searchQuery, s.searchOffset
	var (
		page    pageResult
		saveErr string
	)
	s.schedule(func() {
		if err := store.Assign(network, addr, label, owner); err != nil {
			saveErr = "Could not save the assignment: " + err.Error()
			return
		}
		page = fetchPage(store, network, offset, searching, query, searchOffset)
	}, func() {
		if saveErr != "" {
			s.editErr = saveErr
			return
		}
		s.err = ""
		s.editErr = ""
		s.applyPage(page)
	})
}

// unassign removes the edited assignment and reloads the page. The database
// work runs off the UI thread.
func (s *ipamState) unassign() {
	if s.store == nil {
		s.editErr = "The IPAM database is unavailable."
		return
	}
	if !s.editAddr.IsValid() {
		s.editErr = "Select an address first."
		return
	}
	store := s.store
	network, addr := s.network, s.editAddr
	offset, searching, query, searchOffset := s.offset, s.searching, s.searchQuery, s.searchOffset
	var (
		page    pageResult
		saveErr string
	)
	s.schedule(func() {
		if err := store.Unassign(network, addr); err != nil {
			saveErr = "Could not remove the assignment: " + err.Error()
			return
		}
		page = fetchPage(store, network, offset, searching, query, searchOffset)
	}, func() {
		if saveErr != "" {
			s.editErr = saveErr
			return
		}
		s.err = ""
		s.editErr = ""
		s.applyPage(page)
	})
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

// rename saves the new network name and reloads the list. The database work
// runs off the UI thread.
func (s *ipamState) rename() {
	if s.store == nil {
		s.renameErr = "The IPAM database is unavailable."
		return
	}
	if s.current() == nil {
		return
	}
	store := s.store
	network, name := s.network, s.renameName
	var renameErr string
	s.schedule(func() {
		if _, err := store.RenameNetwork(network, name); err != nil {
			renameErr = "Could not rename the network: " + err.Error()
		}
	}, func() {
		if renameErr != "" {
			s.renameErr = renameErr
			return
		}
		s.cancelRename()
		s.refresh()
	})
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

// deleteNetwork removes the current network and selects another one. The
// database work runs off the UI thread.
func (s *ipamState) deleteNetwork() {
	if s.store == nil {
		s.err = "The IPAM database is unavailable."
		return
	}
	if s.network == 0 {
		return
	}
	store := s.store
	network := s.network
	var delErr string
	s.schedule(func() {
		if err := store.DeleteNetwork(network); err != nil {
			delErr = "Could not delete the network: " + err.Error()
		}
	}, func() {
		if delErr != "" {
			// Close the dialog so the alert behind it is visible.
			s.confirmDelete = false
			s.err = delErr
			return
		}
		s.confirmDelete = false
		s.network = 0
		s.offset = 0
		s.resetSearch()
		s.clearEdit()
		s.cancelRename()
		s.refresh()
	})
}

// saveNetwork stores a new network and makes it the selected one. The
// database work runs off the UI thread; done is called back on the UI
// thread with the saved network or the error.
func (s *ipamState) saveNetwork(cidr, name string, done func(ipam.Network, error)) {
	if s.store == nil {
		done(ipam.Network{}, fmt.Errorf("the IPAM database is unavailable"))
		return
	}
	store := s.store
	var (
		saved ipam.Network
		err   error
	)
	s.schedule(func() {
		saved, err = store.SaveNetwork(cidr, name)
	}, func() {
		if err != nil {
			done(ipam.Network{}, err)
			return
		}
		s.network = saved.ID
		s.offset = 0
		s.resetSearch()
		s.refresh()
		done(saved, nil)
	})
}

// ipamLayout adapts the page to the window: short windows fold the network
// list and tighten the spacing, narrow ones drop duplicated detail.
type ipamLayout struct {
	compact bool
	narrow  bool
	gap     float32
}

func newIPAMLayout(c *myui.Context) ipamLayout {
	w, h := c.Size()
	l := ipamLayout{compact: h < ipamCompactHeight, narrow: w < ipamNarrowWidth}
	l.gap = spaceL
	if l.compact {
		l.gap = spaceS
	}
	return l
}

func (a *App) ipamView(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	l := newIPAMLayout(c)
	myui.Column(c).Grow(1).Gap(l.gap).Children(func() {
		if s.storeErr != "" {
			alert(c, s.storeErr, t.Danger)
			return
		}
		if s.err != "" {
			alert(c, s.err, t.Danger)
		}

		// In address-focus mode the networks card is hidden entirely.
		if !s.addressesFocus {
			a.ipamNetworks(c, l)
		}
		if n := s.current(); n != nil {
			a.ipamNetwork(c, n, l)
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

func (a *App) ipamNetworks(c *myui.Context, l ipamLayout) {
	s := &a.ipam
	t := c.Theme()
	collapsed := s.networksCollapsed(l.compact)
	card(c, func() {
		cardHeader(c, "Saved networks",
			"Choose a network to browse and label its usable addresses.", func() {
				pill(c, fmt.Sprintf("%d saved", len(s.networks)), t.Accent)
				toggleLabel := "Hide"
				if collapsed {
					toggleLabel = "Show"
				}
				myui.Button(c, toggleLabel).OnClick(func() {
					s.toggleNetworks(l.compact)
				})
			})
		if collapsed {
			return
		}
		if len(s.networks) == 0 {
			myui.Text(c, "No networks saved yet. Calculate an IPv4 block and save it.").
				FontSize(12).TextColor(t.TextMuted)
			return
		}

		list := func() {
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
		}

		// Many networks: scroll inside a fixed height so the address grid
		// below always keeps its space.
		if len(s.networks) > networksScrollThreshold {
			myui.Scroll(c).Key("networks-scroll").Height(networksListHeight).Children(list)
		} else {
			list()
		}
	})
}

func (a *App) ipamNetwork(c *myui.Context, n *ipam.Network, l ipamLayout) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Grow(1).Gap(l.gap).Children(func() {
		cardHeader(c, n.Name, fmt.Sprintf("%s  ·  %d used, %d free of %d",
			n.CIDR, s.stats.Used, s.stats.Free, s.stats.Total), func() {
			// The counts are already in the subtitle; the pills only earn
			// their space on wide windows.
			if !l.narrow {
				pill(c, fmt.Sprintf("%d used", s.stats.Used), t.Accent)
				pill(c, fmt.Sprintf("%d free", s.stats.Free), t.Success)
			}

			if s.addressesFocus {
				myui.PrimaryButton(c, "Restore view").OnClick(func() {
					s.addressesFocus = false
				})
				return
			}
			myui.Button(c, "Expand addresses").OnClick(func() {
				s.addressesFocus = true
			})
			myui.Button(c, "Rename network").OnClick(s.beginRename)
			myui.Button(c, "Delete network").TextColor(t.Danger).OnClick(s.beginDelete)
		})

		if s.renaming && !s.addressesFocus {
			inset(c, func() {
				myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
					input := myui.TextInput(c, &s.renameName).
						Label("Network name").
						Placeholder("Office").
						Grow(1).
						MinWidth(160)
					input.OnSubmit(s.rename)
					myui.PrimaryButton(c, "Save name").OnClick(s.rename)
					myui.Button(c, "Cancel").OnClick(s.cancelRename)
				})
				if s.renameErr != "" {
					alert(c, s.renameErr, t.Danger)
				}
			})
		}

		a.ipamToolbar(c, l)
		a.ipamEditor(c)
		a.ipamGrid(c)
	})
}

// ipamToolbar holds the search box and the page controls in one row that
// wraps to a second line only when the window is too narrow for it.
func (a *App) ipamToolbar(c *myui.Context, l ipamLayout) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Gap(spaceS).Children(func() {
		myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
			search := myui.TextInput(c, &s.searchInput).
				Label("Search").
				Placeholder("Search IP, label or owner").
				Grow(1).
				MinWidth(180)
			search.OnSubmit(s.submitSearch)

			// While a load is in flight the buttons stand down; the load
			// applies when it lands and supersedes clicks meanwhile.
			searchButton := myui.PrimaryButton(c, "Search").Key("search").OnClick(s.submitSearch)
			searchButton.Disabled(s.busy)
			if s.searching || s.searchInput != "" {
				myui.Button(c, "Clear").Key("clear").OnClick(s.clearSearch)
			}
			nextFreeButton := myui.Button(c, "Next free").Key("next-free").Tooltip("Select the next free address").OnClick(s.nextFree)
			nextFreeButton.Disabled(s.busy)

			offset, total := s.pager()
			myui.Row(c).Key("pager").Gap(spaceS).AlignItems(myui.Center).Children(func() {
				previous := myui.Button(c, "Prev").Key("prev").OnClick(s.previousPage)
				previous.Disabled(offset == 0 || s.busy)
				myui.Text(c, s.pageLabel(l.narrow)).FontSize(12).TextColor(t.TextMuted)
				next := myui.Button(c, "Next").Key("next").OnClick(s.nextPage)
				next.Disabled(offset+ipamPageSize >= total || s.busy)
			})
		})
		if s.searchMsg != "" {
			alert(c, s.searchMsg, t.Warning)
		}
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
				Grow(1).
				MinWidth(150)
			myui.TextInput(c, &s.editOwner).
				Label("Owner").
				Placeholder("Team or person").
				Grow(1).
				MinWidth(150)
			myui.PrimaryButton(c, "Save assignment").OnClick(s.assign)
			myui.Button(c, "Remove assignment").OnClick(s.unassign)
			if s.searching {
				myui.Button(c, "Show in range").Key("locate").OnClick(s.locateEdit)
			}
		})
		if s.editErr != "" {
			alert(c, s.editErr, t.Danger)
		}
	})
}

func (a *App) ipamGrid(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	shown := s.shown()
	myui.Scroll(c).Grow(1).Key(s.network).Children(func() {
		if len(shown) == 0 {
			if s.searching {
				myui.Text(c, fmt.Sprintf("No assignments match %q.", s.searchQuery)).
					FontSize(12).TextColor(t.TextMuted)
			}
			return
		}
		myui.Grid(c).Columns(columnsFor(c, gridCellWidth, 5)).Gap(spaceS).Children(func() {
			for _, entry := range shown {
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
				if s.editAddr == entry.Addr {
					cell.Background(t.Accent.Alpha(0.18)).BorderColor(t.Accent)
					if s.reveal {
						cell.ScrollIntoView()
					}
				} else if cell.Hovered() {
					cell.Background(t.SurfaceHover).BorderColor(t.Accent.Alpha(0.5))
				}
				cell.OnClick(func() { s.beginEdit(entry) })
			}
		})
	})
	s.reveal = false
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
