package ui

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	mygo "github.com/egoist/mygo"
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

	// mu guards the export result, written from the export goroutine.
	mu        sync.Mutex
	exportMsg string
	exportErr bool
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
	s.exportMsg = ""
	s.exportErr = false
}

// export opens a native save dialog off the UI thread and writes the
// allocation table as CSV, which Excel opens directly.
func (s *vlsmState) export(plan *netcalc.VLSMPlan, invalidate func()) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       "Export allocation",
			DefaultPath: "vlsm-allocation-" + time.Now().Format("2006-01-02_15-04-05") + ".csv",
			Filters:     []mygo.FileFilter{{Name: "CSV (Excel)", Extensions: []string{"csv"}}},
		})
		if path == "" {
			return // dismissed or unavailable
		}
		if err == nil {
			err = os.WriteFile(path, vlsmCSV(plan), 0o644)
		}
		s.setExportResult(err, path, invalidate)
	}()
}

func (s *vlsmState) setExportResult(err error, path string, invalidate func()) {
	s.mu.Lock()
	if err != nil {
		s.exportErr = true
		s.exportMsg = "Could not export: " + err.Error()
	} else {
		s.exportErr = false
		s.exportMsg = "Exported the allocation table to " + path + "."
	}
	s.mu.Unlock()
	invalidate()
}

func (a *App) vlsmView(c *myui.Context) {
	s := &a.vlsm
	t := c.Theme()
	myui.Scroll(c).Grow(1).Children(func() {
		myui.Column(c).Gap(spaceL).Children(func() {
			card(c, func() {
				cardHeader(c, "The block to divide", "Every department is allocated from this network.", nil)
				myui.Row(c).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
					input := myui.TextInput(c, &s.parent).
						Placeholder("192.168.1.0/24").
						Label("Parent network").
						Width(300)
					input.OnSubmit(s.allocate)
					myui.Text(c, "Largest department first, aligned to its block size.").
						FontSize(11.5).TextColor(t.TextMuted)
				})
			})

			card(c, func() {
				cardHeader(c, "Departments", "Name each group and the hosts it needs.", func() {
					myui.Button(c, "Add department").OnClick(s.addRow)
					myui.PrimaryButton(c, "Allocate").OnClick(s.allocate)
				})

				myui.Column(c).Gap(spaceS).Children(func() {
					for i, row := range s.rows {
						row := row
						myui.Row(c).Key(row.id).Gap(spaceS).AlignItems(myui.Center).Wrap().Children(func() {
							number := myui.Box(c).Width(26).Height(26).Radius(8).
								Background(t.SurfacePressed).Shrink(0)
							number.Children(func() {
								myui.Column(c).Fill().Center().Children(func() {
									myui.Text(c, strconv.Itoa(i+1)).FontSize(11).FontWeight(650).TextColor(t.TextMuted)
								})
							})
							myui.TextInput(c, &row.name).
								Placeholder("Department").
								Label("Department " + strconv.Itoa(row.id)).
								Width(230)
							myui.TextInput(c, &row.hosts).
								Placeholder("Hosts").
								Label("Hosts " + strconv.Itoa(row.id)).
								Width(110)
							myui.Button(c, "Remove").
								Label("Remove row " + strconv.Itoa(row.id)).
								OnClick(func() { s.removeRow(row.id) })
						})
					}
				})

				if s.err != "" {
					alert(c, s.err, t.Danger)
				}
			})

			if s.plan != nil {
				a.vlsmResult(c, s.plan)
			}
		})
	})
}

func (a *App) vlsmResult(c *myui.Context, plan *netcalc.VLSMPlan) {
	t := c.Theme()
	s := &a.vlsm
	services := c.Services()
	allocated := uint64(0)
	for _, allocation := range plan.Allocations {
		allocated += uint64(1) << (32 - allocation.Subnet.Prefix)
	}
	total := uint64(1) << (32 - plan.Parent.Prefix)
	free := total - allocated

	card(c, func() {
		cardHeader(c, "Allocation", fmt.Sprintf(
			"%d of %d addresses allocated across %d departments.",
			allocated, total, len(plan.Allocations)), func() {
			pill(c, fmt.Sprintf("%d used", allocated), t.Accent)
			pill(c, fmt.Sprintf("%d free", free), t.Success)
			myui.Button(c, "Export CSV").OnClick(func() {
				s.export(plan, services.Invalidate)
			})
		})

		s.mu.Lock()
		exportMsg, exportErr := s.exportMsg, s.exportErr
		s.mu.Unlock()
		if exportMsg != "" {
			color := t.Success
			if exportErr {
				color = t.Danger
			}
			alert(c, exportMsg, color)
		}

		colors := []myui.Color{t.Accent, t.Success, t.Warning, t.Danger}
		myui.Row(c).Height(10).Gap(3).Children(func() {
			for i, allocation := range plan.Allocations {
				size := uint64(1) << (32 - allocation.Subnet.Prefix)
				myui.Box(c).Grow(float32(size)).Height(10).
					Background(colors[i%len(colors)]).Radius(5)
			}
			if free > 0 {
				myui.Box(c).Grow(float32(free)).Height(10).
					Background(t.SurfacePressed).Radius(5)
			}
		})

		a.vlsmTable(c, plan)
	})
}

func (a *App) vlsmTable(c *myui.Context, plan *netcalc.VLSMPlan) {
	columns := []myui.TableColumn{
		{Title: "Department", Width: 105},
		{Title: "CIDR", Width: 125},
		{Title: "Mask", Width: 115},
		{Title: "Usable range", Width: 195},
		{Title: "Gateway", Width: 112},
		{Title: "Broadcast", Width: 112},
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
		myui.Text(c, text).FontSize(12).SingleLine()
	}).MinHeight(220)
}

// vlsmCSV renders the allocation table as an Excel-compatible CSV document.
func vlsmCSV(plan *netcalc.VLSMPlan) []byte {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf") // UTF-8 BOM so Excel detects the encoding
	w := csv.NewWriter(&b)
	w.Write([]string{"Department", "CIDR", "Mask", "Usable range", "Gateway", "Broadcast", "Usable hosts"})
	for _, allocation := range plan.Allocations {
		w.Write([]string{
			allocation.Name,
			cidrString(allocation.Subnet),
			allocation.Subnet.Mask.String(),
			allocation.Subnet.First.String() + " - " + allocation.Subnet.Last.String(),
			allocation.Subnet.Gateway.String(),
			allocation.Subnet.Broadcast.String(),
			strconv.FormatUint(allocation.Subnet.Usable, 10),
		})
	}
	w.Flush()
	return b.Bytes()
}
