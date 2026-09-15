// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import (
	"reflect"
	"testing"
)

func TestExtractSet(t *testing.T) {
	// 10 = 0b1010 -> bits nas posições 1 e 3
	got := ExtractSet([]uint64{10})
	want := []uint64{1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractSet([10]) = %v, want %v", got, want)
	}

	// duas palavras: palavra 0 com bit 63, palavra 1 com bit 0 -> pos 63 e 64
	got2 := ExtractSet([]uint64{1 << 63, 1})
	want2 := []uint64{63, 64}
	if !reflect.DeepEqual(got2, want2) {
		t.Fatalf("ExtractSet multi-word = %v, want %v", got2, want2)
	}
}
