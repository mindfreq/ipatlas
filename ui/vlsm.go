package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/netcalc"
)

type vlsmRow struct {
	id    int
	name  string
	hosts string
}

type vlsmState struct {
	parent string
	rows   []*vlsmRow
	nextID int
	plan   *netcalc.VLSMPlan
	err    string
	table  myui.ListState
}

func (s *vlsmState) addRow() {
	s.nextID++
	s.rows = append(s.rows, &vlsmRow{id: s.nextID})
	s.invalidate()
}

func (s *vlsmState) removeRow(id int) {
	for i, row := range s.rows {
		if row.id == id {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			break
		}
	}
	s.invalidate()
}

func (s *vlsmState) invalidate() {
	s.plan = nil
	s.err = ""
}

func (s *vlsmState) allocate() {
	s.invalidate()

	parent := strings.TrimSpace(s.parent)
	if parent == "" {
		s.err = "Enter a parent network, such as 192.168.1.0/24."
		return
	}
	if len(s.rows) == 0 {
		s.err = "Add at least one department."
		return
	}

	requests := make([]netcalc.VLSMRequest, 0, len(s.rows))
	for i, row := range s.rows {
		name := strings.TrimSpace(row.name)
		if name == "" {
			name = fmt.Sprintf("Department %d", i+1)
		}
		hostsText := strings.TrimSpace(row.hosts)
		if hostsText == "" {
			s.err = fmt.Sprintf("Enter a host count for %s.", name)
			return
		}
		hosts, err := strconv.ParseUint(hostsText, 10, 64)
		if err != nil {
			s.err = fmt.Sprintf("Hosts for %s must be a whole number.", name)
			return
		}
		requests = append(requests, netcalc.VLSMRequest{Name: name, Hosts: hosts})
	}

	plan, err := netcalc.AllocateVLSM(parent, requests)
	if err != nil {
		switch {
		case errors.Is(err, netcalc.ErrNoVLSMRequests):
			s.err = "Add at least one department."
		case errors.Is(err, netcalc.ErrInvalidHosts):
			s.err = "Host counts must be at least 1."
		case errors.Is(err, netcalc.ErrVLSMNoSpace):
			s.err = "Not enough space in the parent network. Reduce a host count or use a larger parent."
		case errors.Is(err, netcalc.ErrNotIPv4):
			s.err = "The parent network must be an IPv4 CIDR, such as 192.168.1.0/24."
		case errors.Is(err, netcalc.ErrInvalidCIDR):
			s.err = "The parent network is not a valid CIDR."
		default:
			s.err = err.Error()
		}
		return
	}
	s.plan = &plan
}

func (a *App) vlsmView(c *myui.Context) {
	s := &a.vlsm
	t := c.Theme()
	myui.Column(c).Fill().Gap(12).Children(func() {
		myui.Text(c, "VLSM planner").FontSize(20).Bold()
		myui.Text(c, "Split a parent IPv4 network into departments, largest block first.").TextColor(t.TextMuted)
		myui.Row(c).Gap(8).AlignItems(myui.Center).Children(func() {
			myui.TextInput(c, &s.parent).Placeholder("192.168.1.0/24").Label("Parent network").Width(280)
		})
		myui.Column(c).Gap(8).Children(func() {
			for _, row := range s.rows {
				row := row
				myui.Row(c).Key(row.id).Gap(8).AlignItems(myui.Center).Children(func() {
					myui.TextInput(c, &row.name).Placeholder("Department").Label("Department " + strconv.Itoa(row.id)).Width(220)
					myui.TextInput(c, &row.hosts).Placeholder("Hosts").Label("Hosts " + strconv.Itoa(row.id)).Width(100)
					myui.Button(c, "Remove").Label("Remove row " + strconv.Itoa(row.id)).OnClick(func() {
						s.removeRow(row.id)
					})
				})
			}
		})
		myui.Row(c).Gap(8).Children(func() {
			myui.Button(c, "Add department").OnClick(s.addRow)
			myui.PrimaryButton(c, "Allocate").OnClick(s.allocate)
		})
		if s.err != "" {
			myui.Text(c, s.err).TextColor(t.Danger).Selectable()
		}
		if s.plan != nil {
			a.vlsmResult(c, s.plan)
		}
	})
}

func (a *App) vlsmResult(c *myui.Context, plan *netcalc.VLSMPlan) {
	t := c.Theme()
	allocated := uint64(0)
	for _, allocation := range plan.Allocations {
		allocated += uint64(1) << (32 - allocation.Subnet.Prefix)
	}
	total := uint64(1) << (32 - plan.Parent.Prefix)
	free := total - allocated

	myui.Column(c).Grow(1).Gap(10).Children(func() {
		myui.Text(c, fmt.Sprintf(
			"%d departments, %d of %d addresses allocated (%d free)",
			len(plan.Allocations), allocated, total, free,
		)).TextColor(t.TextMuted)

		colors := []myui.Color{t.Accent, t.Success, t.Warning, t.Danger}
		myui.Row(c).Height(14).Gap(2).Children(func() {
			for i, allocation := range plan.Allocations {
				size := uint64(1) << (32 - allocation.Subnet.Prefix)
				myui.Box(c).Grow(float32(size)).Height(14).Background(colors[i%len(colors)])
			}
			if free > 0 {
				myui.Box(c).Grow(float32(free)).Height(14).Background(t.SurfacePressed)
			}
		})

		a.vlsmTable(c, plan)
	})
}

func (a *App) vlsmTable(c *myui.Context, plan *netcalc.VLSMPlan) {
	columns := []myui.TableColumn{
		{Title: "Department", Width: 150},
		{Title: "CIDR", Width: 140},
		{Title: "Mask", Width: 140},
		{Title: "Usable range", Width: 220},
		{Title: "Gateway", Width: 130},
		{Title: "Broadcast", Width: 140},
	}
	myui.Table(c, &a.vlsm.table, columns, len(plan.Allocations), func(row, col int) {
		allocation := plan.Allocations[row]
		var text string
		switch col {
		case 0:
			text = allocation.Name
		case 1:
			text = cidrString(allocation.Subnet)
		case 2:
			text = allocation.Subnet.Mask.String()
		case 3:
			text = allocation.Subnet.First.String() + " - " + allocation.Subnet.Last.String()
		case 4:
			text = allocation.Subnet.Gateway.String()
		case 5:
			text = allocation.Subnet.Broadcast.String()
		}
		myui.Text(c, text).SingleLine()
	}).Grow(1)
}
