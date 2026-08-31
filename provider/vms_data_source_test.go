package provider

import (
	"reflect"
	"testing"
)

func filterFixtures() []vmItem {
	return []vmItem{
		{ServiceID: "1", Hostname: "web", Status: "active", Package: "NVMe VPS - Standard"},
		{ServiceID: "2", Hostname: "db", Status: "active", Package: "Dev VPS - Starter"},
		{ServiceID: "3", Hostname: "old", Status: "canceled", Package: "SSD VPS - Standard"},
	}
}

func TestFilterVMItems_whenNoFilters(t *testing.T) {
	got := filterVMItems(filterFixtures(), vmFilter{})
	if len(got) != 3 {
		t.Errorf("expected all 3 VMs unfiltered, got %d", len(got))
	}
}

func TestFilterVMItems_whenStatus(t *testing.T) {
	got := filterVMItems(filterFixtures(), vmFilter{Status: "active"})
	if len(got) != 2 {
		t.Fatalf("expected 2 active VMs, got %d", len(got))
	}
	for _, vm := range got {
		if vm.Status != "active" {
			t.Errorf("got non-active VM %q in status-filtered result", vm.Hostname)
		}
	}
}

func TestFilterVMItems_whenZone(t *testing.T) {
	got := filterVMItems(filterFixtures(), vmFilter{Zone: "cherryvale"})
	if len(got) != 1 || got[0].Hostname != "db" {
		t.Errorf("expected only the Dev VM for cherryvale, got %+v", got)
	}
	katy := filterVMItems(filterFixtures(), vmFilter{Zone: "katy"})
	if len(katy) != 2 {
		t.Errorf("expected 2 Katy VMs, got %+v", katy)
	}
}

func TestFilterVMItems_whenPackageSubstring(t *testing.T) {
	got := filterVMItems(filterFixtures(), vmFilter{Package: "dev vps"})
	if len(got) != 1 || got[0].Hostname != "db" {
		t.Errorf("expected case-insensitive package match on the Dev VM, got %+v", got)
	}
}

func TestFilterVMItems_whenCombined(t *testing.T) {
	got := filterVMItems(filterFixtures(), vmFilter{Status: "canceled", Zone: "katy"})
	want := []string{"old"}
	var names []string
	for _, vm := range got {
		names = append(names, vm.Hostname)
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("combined filter: got %v, want %v", names, want)
	}
}
