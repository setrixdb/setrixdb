// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

// Command ringbench — mede a propriedade central do consistent hash ring:
// entrar/sair um nó remapeia apenas ~1/(N+1) das chaves (vs ~N/(N+1) do módulo
// ingênuo), e o balanceamento fica próximo do ideal.
//
// Uso: go run ./cmd/ringbench
package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func routeAll(r *addb.Ring, keys []uint64) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i], _ = r.Route(k)
	}
	return out
}

func pctChanged(a, b []string) float64 {
	n := 0
	for i := range a {
		if a[i] != b[i] {
			n++
		}
	}
	return 100 * float64(n) / float64(len(a))
}

func maxShare(shards []string) float64 {
	c := map[string]int{}
	for _, s := range shards {
		c[s]++
	}
	mx := 0
	for _, v := range c {
		if v > mx {
			mx = v
		}
	}
	return 100 * float64(mx) / float64(len(shards))
}

func main() {
	replicas := flag.Int("replicas", 100, "pontos virtuais por nó")
	flag.Parse()

	const keys = 1_000_000
	ks := make([]uint64, keys)
	r := rand.New(rand.NewSource(1))
	for i := range ks {
		ks[i] = r.Uint64()
	}

	fmt.Printf("== SetrixDB hash ring — churn (1M IDs, %d réplicas/nó) ==\n", *replicas)
	fmt.Printf("%-7s %-11s %-11s %-12s %-14s\n", "nodes", "add→remap", "del→remap", "balance máx", "módulo add→remap")
	for _, N := range []int{4, 8, 16, 64} {
		ring := addb.NewRing(*replicas)
		for i := 0; i < N; i++ {
			ring.AddNode(fmt.Sprintf("n%d", i))
		}
		b0 := routeAll(ring, ks)

		ring.AddNode("new")
		b1 := routeAll(ring, ks)
		addR := pctChanged(b0, b1)
		bal := maxShare(b1)

		ring.RemoveNode("new")
		b2 := routeAll(ring, ks)
		delR := pctChanged(b1, b2)

		modR := 100 * float64(N) / float64(N+1) // módulo: quase tudo remapeia
		fmt.Printf("%-7d %-11.1f %-11.1f %-12.0f %-14.1f\n", N, addR, delR, bal, modR)
	}
	fmt.Println("\n(ideal: add/del remapeia ~100/(N+1)%, balance máx ~100/(N+1)%)")
}
