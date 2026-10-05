// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package pkgbuild

import "testing"

func TestHashAlgosTrackedPerArray(t *testing.T) {
	pb := &PKGBUILD{}
	pb.Init()

	if err := pb.AddItem("sha512sums", []string{"a"}); err != nil {
		t.Fatal(err)
	}

	if err := pb.AddItem("b2sums_x86_64", []string{"b"}); err != nil {
		t.Fatal(err)
	}

	pb.Finalize()

	if len(pb.HashSums) != 2 || len(pb.HashAlgos) != 2 ||
		pb.HashAlgos[0] != "sha512sums" || pb.HashAlgos[1] != "b2sums" {
		t.Errorf("HashSums=%v HashAlgos=%v", pb.HashSums, pb.HashAlgos)
	}
}
