// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package pkgbuild

import (
	"slices"
	"testing"
)

func TestAddItem_BaseDoesNotOverrideDistroSpecific(t *testing.T) {
	pb := &PKGBUILD{FullDistroName: "ubuntu_focal", Distro: "ubuntu"}
	pb.Init()

	if err := pb.AddItem("depends__ubuntu", []string{"ubuntu-dep"}); err != nil {
		t.Fatalf("AddItem(depends__ubuntu) error: %v", err)
	}

	if err := pb.AddItem("pkgdesc__ubuntu", "ubuntu desc"); err != nil {
		t.Fatalf("AddItem(pkgdesc__ubuntu) error: %v", err)
	}

	// Base directives declared AFTER the distro-specific ones must not win.
	if err := pb.AddItem("depends", []string{"base-dep"}); err != nil {
		t.Fatalf("AddItem(depends) error: %v", err)
	}

	if err := pb.AddItem("pkgdesc", "base desc"); err != nil {
		t.Fatalf("AddItem(pkgdesc) error: %v", err)
	}

	if !slices.Equal(pb.Depends, []string{"ubuntu-dep"}) {
		t.Errorf("Depends = %v, want [ubuntu-dep]", pb.Depends)
	}

	if pb.PkgDesc != "ubuntu desc" {
		t.Errorf("PkgDesc = %q, want %q", pb.PkgDesc, "ubuntu desc")
	}
}

func TestAddItem_BaseBeforeDistroSpecific(t *testing.T) {
	pb := &PKGBUILD{FullDistroName: "ubuntu_focal", Distro: "ubuntu"}
	pb.Init()

	if err := pb.AddItem("depends", []string{"base-dep"}); err != nil {
		t.Fatalf("AddItem(depends) error: %v", err)
	}

	if err := pb.AddItem("depends__ubuntu", []string{"ubuntu-dep"}); err != nil {
		t.Fatalf("AddItem(depends__ubuntu) error: %v", err)
	}

	if !slices.Equal(pb.Depends, []string{"ubuntu-dep"}) {
		t.Errorf("Depends = %v, want [ubuntu-dep]", pb.Depends)
	}
}

func TestAddItem_BaseSourceStillAccumulatesAfterArchSpecific(t *testing.T) {
	pb := &PKGBUILD{FullDistroName: "ubuntu_focal", Distro: "ubuntu", TargetArch: "aarch64"}
	pb.Init()

	if err := pb.AddItem("source_aarch64", []string{"arch-src"}); err != nil {
		t.Fatalf("AddItem(source_aarch64) error: %v", err)
	}

	if err := pb.AddItem("source", []string{"base-src"}); err != nil {
		t.Fatalf("AddItem(source) error: %v", err)
	}

	if !slices.Equal(pb.SourceURI, []string{"base-src"}) {
		t.Errorf("SourceURI = %v, want [base-src]", pb.SourceURI)
	}
}

func TestParseSplitOverrides_ExpandsPKGBUILDVariables(t *testing.T) {
	pb := &PKGBUILD{PkgName: "foo-libs", PkgBase: "foo", PkgVer: "1.2.3", PkgRel: "4"}
	pb.Init()

	pb.PkgName = "foo-libs"
	pb.PkgBase = "foo"
	pb.PkgVer = "1.2.3"
	pb.PkgRel = "4"
	pb.CustomVariables["_extra"] = "bar"

	funcBody := "depends=(\"${pkgbase}=${pkgver}-${pkgrel}\" \"${pkgname}-data\" \"${_extra}\")\n" +
		"pkgdesc=\"${pkgbase} libs ${pkgver}\"\n"

	if err := pb.ParseSplitOverrides(funcBody); err != nil {
		t.Fatalf("ParseSplitOverrides() error: %v", err)
	}

	want := []string{"foo=1.2.3-4", "foo-libs-data", "bar"}
	if !slices.Equal(pb.Depends, want) {
		t.Errorf("Depends = %v, want %v", pb.Depends, want)
	}

	if pb.PkgDesc != "foo libs 1.2.3" {
		t.Errorf("PkgDesc = %q, want %q", pb.PkgDesc, "foo libs 1.2.3")
	}
}

func TestParseSplitOverrides_ReportsExpansionError(t *testing.T) {
	pb := &PKGBUILD{}
	pb.Init()

	// Unsupported syntax in an override must be surfaced, not silently dropped.
	if err := pb.ParseSplitOverrides("pkgdesc=\"$(date)\"\n"); err == nil {
		t.Error("ParseSplitOverrides() expected error for command substitution")
	}
}
