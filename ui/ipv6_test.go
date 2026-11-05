package ui

import (
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"ipatlas/internal/netcalc"
)

func TestIPv6Inspect(t *testing.T) {
	s := ipv6State{address: "2001:db8::1"}
	s.inspect()

	if s.addressErr != "" {
		t.Fatalf("error = %q, want none", s.addressErr)
	}
	if s.info == nil {
		t.Fatal("info = nil, want an address")
	}
	if s.info.Compressed != "2001:db8::1" {
		t.Errorf("compressed = %q", s.info.Compressed)
	}
	if s.info.Expanded != "2001:0db8:0000:0000:0000:0000:0000:0001" {
		t.Errorf("expanded = %q", s.info.Expanded)
	}
	if s.info.Kind != netcalc.IPv6GlobalUnicast {
		t.Errorf("kind = %s, want global unicast", s.info.Kind)
	}
}

func TestIPv6InspectErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "Enter an IPv6 address"},
		{name: "ipv4", input: "192.168.1.1", want: "not an IPv6 address"},
		{name: "garbage", input: "not-an-ip", want: "not valid IPv6"},
		{name: "prefix", input: "2001:db8::/64", want: "not valid IPv6"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := ipv6State{address: tc.input, info: &netcalc.IPv6Info{}}
			s.inspect()
			if s.info != nil {
				t.Errorf("info = %+v, want nil", s.info)
			}
			if !strings.Contains(s.addressErr, tc.want) {
				t.Errorf("error = %q, want it to contain %q", s.addressErr, tc.want)
			}
		})
	}
}

func TestIPv6SplitPages(t *testing.T) {
	s := ipv6State{parent: "2001:db8::/48", childBits: "64"}
	s.loadPage(0)

	if s.splitErr != "" {
		t.Fatalf("error = %q, want none", s.splitErr)
	}
	if s.split == nil {
		t.Fatal("split = nil")
	}
	if s.page.Total != 65536 {
		t.Errorf("total = %d, want 65536", s.page.Total)
	}
	if len(s.page.Prefixes) != ipv6PageSize {
		t.Errorf("page size = %d, want %d", len(s.page.Prefixes), ipv6PageSize)
	}
	if got := s.page.Prefixes[0].String(); got != "2001:db8::/64" {
		t.Errorf("first prefix = %s, want 2001:db8::/64", got)
	}

	s.nextPage()
	if s.page.Offset != ipv6PageSize {
		t.Fatalf("offset after next = %d, want %d", s.page.Offset, ipv6PageSize)
	}
	if got := s.page.Prefixes[0].String(); got != "2001:db8:0:100::/64" {
		t.Errorf("first prefix of page 2 = %s, want 2001:db8:0:100::/64", got)
	}

	s.previousPage()
	if s.page.Offset != 0 {
		t.Errorf("offset after previous = %d, want 0", s.page.Offset)
	}
}

func TestIPv6SplitSinglePage(t *testing.T) {
	s := ipv6State{parent: "2001:db8::/64", childBits: "64"}
	s.loadPage(0)
	if s.splitErr != "" {
		t.Fatalf("error = %q, want none", s.splitErr)
	}
	if s.page.Total != 1 || len(s.page.Prefixes) != 1 {
		t.Fatalf("page = %+v, want one prefix", s.page)
	}
	s.nextPage()
	if s.page.Offset != 0 {
		t.Errorf("offset after next on last page = %d, want 0", s.page.Offset)
	}
	s.previousPage()
	if s.page.Offset != 0 {
		t.Errorf("offset after previous on first page = %d, want 0", s.page.Offset)
	}
}

func TestIPv6SplitErrors(t *testing.T) {
	tests := []struct {
		name      string
		parent    string
		childBits string
		offset    uint64
		want      string
	}{
		{name: "empty prefix", childBits: "64", want: "Enter a prefix"},
		{name: "empty child", parent: "2001:db8::/48", want: "Enter a new prefix length"},
		{name: "non numeric child", parent: "2001:db8::/48", childBits: "sixty-four", want: "whole number"},
		{name: "ipv4 prefix", parent: "192.168.0.0/24", childBits: "64", want: "must be IPv6"},
		{name: "invalid prefix", parent: "nope", childBits: "64", want: "not a valid CIDR"},
		{name: "child too short", parent: "2001:db8::/48", childBits: "40", want: "between the parent length and 128"},
		{name: "too many subnets", parent: "::/0", childBits: "64", want: "too many subnets"},
		{name: "page out of range", parent: "2001:db8::/48", childBits: "64", offset: 65536, want: "out of range"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := ipv6State{parent: tc.parent, childBits: tc.childBits}
			s.loadPage(tc.offset)
			if s.split != nil {
				t.Errorf("split = %+v, want nil", s.split)
			}
			if !strings.Contains(s.splitErr, tc.want) {
				t.Errorf("error = %q, want it to contain %q", s.splitErr, tc.want)
			}
		})
	}
}

func TestIPv6AddressUI(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("IPv6"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("IPv6 tools") {
		t.Fatalf("texts %q do not show IPv6 tools", tt.Texts())
	}
	if err := tt.Click("IPv6 address"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2001:db8::1")
	if err := tt.Click("Inspect"); err != nil {
		t.Fatal(err)
	}

	if a.ipv6.info == nil {
		t.Fatalf("info = nil, texts %q", tt.Texts())
	}
	for _, want := range []string{
		"2001:0db8:0000:0000:0000:0000:0000:0001",
		"global unicast",
		"Compressed",
		"Expanded",
		"Type",
	} {
		if !hasExactText(tt, want) {
			t.Errorf("texts %q do not contain exact text %q", tt.Texts(), want)
		}
	}
}

func TestIPv6AddressUIError(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("IPv6"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("IPv6 address"); err != nil {
		t.Fatal(err)
	}
	tt.Type("192.168.1.1")
	if err := tt.Click("Inspect"); err != nil {
		t.Fatal(err)
	}

	if a.ipv6.info != nil {
		t.Errorf("info = %+v, want nil", a.ipv6.info)
	}
	if !tt.HasText("not an IPv6 address") {
		t.Errorf("texts %q do not show the error", tt.Texts())
	}
}

func TestIPv6SplitterUI(t *testing.T) {
	a := New()
	tt := myui.NewTester(a.View, 1100, 720)

	if err := tt.Click("IPv6"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("IPv6 prefix"); err != nil {
		t.Fatal(err)
	}
	tt.Type("2001:db8::/48")
	if err := tt.Click("New prefix length"); err != nil {
		t.Fatal(err)
	}
	tt.Type("64")
	if err := tt.Click("Split"); err != nil {
		t.Fatal(err)
	}

	if a.ipv6.split == nil {
		t.Fatalf("split = nil, texts %q", tt.Texts())
	}
	if len(a.ipv6.page.Prefixes) != ipv6PageSize {
		t.Errorf("page size = %d, want %d", len(a.ipv6.page.Prefixes), ipv6PageSize)
	}
	for _, want := range []string{
		"2001:db8::/64",
		"2001:db8:0:ff::/64",
	} {
		if !hasExactText(tt, want) {
			t.Errorf("texts %q do not contain exact text %q", tt.Texts(), want)
		}
	}
	if !tt.HasText("Page 1 of 256") {
		t.Errorf("texts %q do not show page 1", tt.Texts())
	}

	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	if a.ipv6.page.Offset != ipv6PageSize {
		t.Errorf("offset after Next = %d, want %d", a.ipv6.page.Offset, ipv6PageSize)
	}
	if !hasExactText(tt, "2001:db8:0:100::/64") {
		t.Errorf("texts %q do not show the first prefix of page 2", tt.Texts())
	}
	if !tt.HasText("Page 2 of 256") {
		t.Errorf("texts %q do not show page 2", tt.Texts())
	}

	if err := tt.Click("Previous"); err != nil {
		t.Fatal(err)
	}
	if a.ipv6.page.Offset != 0 {
		t.Errorf("offset after Previous = %d, want 0", a.ipv6.page.Offset)
	}
	if !tt.HasText("Page 1 of 256") {
		t.Errorf("texts %q do not show page 1 after Previous", tt.Texts())
	}
}
