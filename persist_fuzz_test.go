// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package setrixdb

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzReadFile garante que nenhuma entrada malformada causa panic e que,
// quando um conjunto é lido, os IDs vêm ordenados.
func FuzzReadFile(f *testing.F) {
	dir := f.TempDir()
	seed := filepath.Join(dir, "seed.sxset")
	if err := NewSet(1, 2, 100).WriteFile(seed); err != nil {
		f.Fatal(err)
	}
	if b, err := os.ReadFile(seed); err == nil {
		f.Add(b)
	}
	f.Add([]byte("nao-e-sxset"))
	f.Add([]byte("SXSET1"))
	f.Add([]byte("SXSET1S"))
	f.Add([]byte("SXSET1X"))
	f.Add([]byte("SXSET1S\x00\x00\x00\x00\x00\x00\x00\x05"))       // cabeçalho diz 5 IDs, sem corpo
	f.Add([]byte("SXSET1S\xff\xff\xff\xff\xff\xff\xff\xff"))       // n gigante (overflow?)

	f.Fuzz(func(t *testing.T, data []byte) {
		p := filepath.Join(t.TempDir(), "f.sxset")
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Skip()
		}
		set, err := ReadFile(p)
		if err != nil {
			return // erro é aceitável; panic não
		}
		ids := set.IDs()
		for i := 1; i < len(ids); i++ {
			if ids[i-1] > ids[i] {
				t.Fatalf("IDs fora de ordem: %d > %d", ids[i-1], ids[i])
			}
		}
	})
}
