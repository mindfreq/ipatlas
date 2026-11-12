package ui

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/ipam"
)

const ipamPageSize = 256

// searchDebounce is the typing pause before the live search runs, so a
// burst of keystrokes runs one database search, not one per character.
const searchDebounce = 300 * time.Millisecond

const (
	// With more saved networks than this, the list scrolls inside a fixed
	// height instead of growing and pushing the address grid off screen.
	networksScrollThreshold = 6
	networksListHeight      = 120

	// Below these window sizes the page switches to a tighter layout.
	ipamCompactHeight = 720
	ipamNarrowWidth   = 1000
)

// pencilIcon is the small edit affordance beside the network name; parsed
// once, never in a frame.
var pencilIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.85 2.85 0 1 1 4 4L7.5 20.5 2 22l1.5-5Z"/><path d="m15 5 4 4"/></svg>`))

// trashIcon is the delete affordance beside it.
var trashIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><path d="M10 11v6M14 11v6"/></svg>`))

// saveIcon is the assignment's save affordance beside the edit fields.
var saveIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2Z"/><path d="M17 21v-8H7v8"/><path d="M7 3v5h8"/></svg>`))

// chevronLeftIcon and chevronRightIcon page the address grid back and forth.
var chevronLeftIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>`))
var chevronRightIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`))

// expandIcon gives the address grid the page's full width, and restoreIcon
// brings the networks list back.
var expandIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6"/><path d="M9 21H3v-6"/><path d="m21 3-7 7"/><path d="m3 21 7-7"/></svg>`))
var restoreIcon = myui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 14h6v6"/><path d="M20 10h-6V4"/><path d="m14 10 7-7"/><path d="m3 21 7-7"/></svg>`))

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
	// instead of a page of the address range. The search runs live: what
	// is typed settles for searchDebounce, then the matches load.
	searchInput  string // text in the search box
	searchQuery  string // the text query that produced results
	searching    bool
	searchOffset uint64      // index among the matches, not an address offset
	searchTimer  *time.Timer // the pending live search, nil when none waits
	results      []ipam.AddressEntry
	matches      uint64
	searchMsg    string

	// pageInput is the page number typed in the pager's box; it follows
	// the page the grid shows until the user edits it.
	pageInput string

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

// runSearchDebounce arms the live search's pause: it runs fn on the UI
// thread once delay has passed without another keystroke. It is a variable
// so tests run fn right away and keep the state deterministic.
var runSearchDebounce = func(delay time.Duration, fn func()) *time.Timer {
	return time.AfterFunc(delay, func() { mygo.RunOnMain(fn) })
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
		s.pageInput = fmt.Sprintf("%d", p.searchOffset/ipamPageSize+1)
	} else {
		s.entries = p.entries
		s.offset = p.offset
		s.pageInput = fmt.Sprintf("%d", p.offset/ipamPageSize+1)
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

// jumpToPage goes to the page typed in the pager's box, clamped to the
// pages there are.
func (s *ipamState) jumpToPage() {
	n, err := strconv.Atoi(strings.TrimSpace(s.pageInput))
	if err != nil {
		return
	}
	_, total := s.pager()
	pages := int(pageCount(total))
	if pages == 0 {
		return
	}
	n = min(max(n, 1), pages)
	s.setOffset(uint64(n-1) * ipamPageSize)
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
	s.stopSearchTimer()
	s.searching = false
	s.searchInput = ""
	s.searchQuery = ""
	s.searchOffset = 0
	s.results = nil
	s.matches = 0
	s.searchMsg = ""
}

// stopSearchTimer drops a live search that has not run yet, so it cannot
// fire after the state moved on, as to another network.
func (s *ipamState) stopSearchTimer() {
	if s.searchTimer != nil {
		s.searchTimer.Stop()
		s.searchTimer = nil
	}
}

// searchEdited records an edit in the search box and arms the debounced
// live search: each keystroke pushes the run back, so a burst of typing
// runs one search once the text settles.
func (s *ipamState) searchEdited() {
	s.stopSearchTimer()
	s.searchTimer = runSearchDebounce(searchDebounce, s.runDueSearch)
}

// runDueSearch is what the pause fires: it runs what settled in the box.
func (s *ipamState) runDueSearch() {
	s.searchTimer = nil
	s.liveSearch()
}

func (s *ipamState) clearSearch() {
	s.resetSearch()
	if s.store != nil && s.current() != nil {
		s.loadPage()
	}
}

// liveSearch runs what settled in the search box: empty text clears the
// search, anything else searches the assignments as plain text. It never
// jumps, so a half-typed address cannot send the grid elsewhere.
func (s *ipamState) liveSearch() {
	query := strings.TrimSpace(s.searchInput)
	if query == "" {
		if s.searching || s.searchQuery != "" {
			s.clearSearch()
		}
		return
	}
	s.searchText(query)
}

// searchText runs query as an assignment search from the first page of
// matches. The database work runs off the UI thread.
func (s *ipamState) searchText(query string) {
	if s.store == nil || s.current() == nil {
		return
	}
	s.searchMsg = ""
	s.searching = true
	s.searchQuery = query
	s.searchOffset = 0
	s.loadPage()
}

// submitSearch runs what is in the search box as Enter submits it: a full
// IPv4 address jumps to its page, any other text searches the assignments,
// empty text clears the search. A live search still pending is dropped.
func (s *ipamState) submitSearch() {
	s.stopSearchTimer()
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
	s.searchText(query)
}

// jumpTo shows the page that holds addr and selects it. The database work
// runs off the UI thread.
func (s *ipamState) jumpTo(addr netip.Addr) {
	s.stopSearchTimer()
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
	s.stopSearchTimer()
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
		// The counts sit under the name again; the search group takes
		// their old place at the far end, level with the name and icons.
		cardHeaderStart(c, n.Name, fmt.Sprintf("%s  ·  %d used, %d free of %d",
			n.CIDR, s.stats.Used, s.stats.Free, s.stats.Total), func() {
			// The search field runs live: results show as the text
			// settles, Enter runs it straight away (a full IPv4 address
			// then jumps to its page), and the field's own clear button
			// or Escape empties it and returns to the address range.
			// The next-free shortcut stands beside it, standing down
			// while a load is in flight.
			search := myui.SearchField(c, &s.searchInput).
				Label("Search").
				Placeholder("Search IP, label or owner").
				Width(260)
			search.OnSubmit(s.submitSearch)
			if search.Changed() {
				s.searchEdited()
			}
			nextFreeButton := myui.Button(c, "Next free").Key("next-free").Tooltip("Select the next free address").OnClick(s.nextFree)
			nextFreeButton.Disabled(s.busy)

			// Last in the group, so it sits at the far right.
			myui.Row(c).Key("expand-wrap").Padding(5, 0, 0, 0).Children(func() {
				if s.addressesFocus {
					iconButtonPrimary(c, "restore-view", "Restore view", restoreIcon, func() {
						s.addressesFocus = false
					})
				} else {
					iconButton(c, "expand-addresses", "Expand addresses", expandIcon, t.Text, false, func() {
						s.addressesFocus = true
					})
				}
			})
		}, func() {
			// The pencil beside the network name opens the rename editor,
			// and the trash next to it asks before deleting the network.
			iconButton(c, "rename-network", "Rename network", pencilIcon, t.Text, false, s.beginRename)
			iconButton(c, "delete-network", "Delete network", trashIcon, t.Danger, false, s.beginDelete)
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

		a.ipamToolbar(c)
		a.ipamEditor(c)
		a.ipamGrid(c)
	})
}

// ipamToolbar holds the page controls below the search group in the
// network's header.
func (a *App) ipamToolbar(c *myui.Context) {
	s := &a.ipam
	t := c.Theme()
	myui.Column(c).Gap(spaceS).Children(func() {
		offset, total := s.pager()
		if pageCount(total) > 1 {
			// The arrows show only for a range that actually pages,
			// with a small box to type a page and jump straight to it.
			// The section grows over its line's leftover and hugs the
			// end, a little padding keeping it off the row's edge.
			myui.Row(c).Key("pager").Gap(spaceS).AlignItems(myui.Center).
				Grow(1).Justify(myui.End).Padding(0, 8, 0, 0).Children(func() {
				iconButton(c, "prev", "Prev", chevronLeftIcon, t.Text, offset == 0 || s.busy, s.previousPage)
				c.Key("page-jump")
				page := myui.TextInput(c, &s.pageInput).
					Label("Page").
					Placeholder("1").
					Width(64)
				page.OnSubmit(s.jumpToPage)
				myui.Text(c, fmt.Sprintf("/ %d", pageCount(total))).
					FontSize(12).TextColor(t.TextMuted)
				iconButton(c, "next", "Next", chevronRightIcon, t.Text, offset+ipamPageSize >= total || s.busy, s.nextPage)
			})
		}
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
		// The title carries the two fields and their icon actions: save
		// keeps the assignment, the trash removes it.
		myui.Row(c).Key(s.editAddr.String()).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
			myui.Text(c, "Edit "+s.editAddr.String()).FontSize(13).FontWeight(650)
			myui.TextInput(c, &s.editLabel).
				Label("Label").
				Placeholder("Printer, gateway...").
				Grow(1).
				MinWidth(120)
			myui.TextInput(c, &s.editOwner).
				Label("Owner").
				Placeholder("Team or person").
				Grow(1).
				MinWidth(120)
			iconButtonPrimary(c, "save-assignment", "Save assignment", saveIcon, s.assign)
			iconButton(c, "remove-assignment", "Remove assignment", trashIcon, t.Danger, false, s.unassign)
		})
		if s.searching {
			myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Children(func() {
				myui.Button(c, "Show in range").Key("locate").OnClick(s.locateEdit)
			})
		}
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
