// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

// Command ringclusterdemo — integra o consistent hash ring ao cluster: cada
// shard é atribuído a um nó pelo anel (topologia dinâmica). Mede a correção e o
// remapeamento de shards quando um nó entra.
//
// Uso: go run ./cmd/ringclusterdemo -shards 12 -replicas 500
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/cluster"
	"github.com/setrixdb/setrixdb/internal/simd"
)

func buildWords(W int, density float64, seed int64) []uint64 {
	w := make([]uint64, W)
	r := rand.New(rand.NewSource(seed))
	n := int(float64(W) * 64 * density)
	for i := 0; i < n; i++ {
		w[r.Intn(W)] |= 1 << (r.Intn(64))
	}
	return w
}

// ownersFor devolve o dono de cada shard para um conjunto de nós.
func ownersFor(addrs []string, S, replicas int) []string {
	ring := addb.NewRing(replicas)
	for _, a := range addrs {
		ring.AddNode(a)
	}
	out := make([]string, S)
	for s := 0; s < S; s++ {
		out[s], _ = ring.Route(uint64(s))
	}
	return out
}

func main() {
	shards := flag.Int("shards", 12, "número de shards")
	replicas := flag.Int("replicas", 500, "pontos virtuais por nó")
	nnodes := flag.Int("nodes", 3, "nós locais")
	universe := flag.Int("universe", 1<<24, "universo em bits")
	flag.Parse()

	W := *universe / 64
	fmt.Printf("== SetrixDB ring+cluster — universo 2^%d, %d shards, %d nós, %d réplicas ==\n\n",
		log2(*universe), *shards, *nnodes, *replicas)

	// nós locais
	var addrs []string
	var locals []*cluster.Node
	for i := 0; i < *nnodes; i++ {
		nd := cluster.NewNode(fmt.Sprintf("127.0.0.1:%d", 19300+i))
		addr, err := nd.Listen()
		if err != nil {
			fmt.Println("erro:", err)
			return
		}
		addrs = append(addrs, addr)
		locals = append(locals, nd)
	}
	defer func() {
		for _, nd := range locals {
			nd.Close()
		}
	}()

	A := buildWords(W, 0.10, 1)
	B := buildWords(W, 0.05, 7)

	// baseline local
	t := time.Now()
	localCnt := simd.AndPopcount(A, B)
	dLocal := time.Since(t)

	sc, err := cluster.NewShardedCluster(addrs, W, *shards, *replicas)
	if err != nil {
		fmt.Println("erro:", err)
		return
	}
	defer sc.Close()

	// distribuição de shards por nó
	perNode := map[string]int{}
	for _, o := range sc.Owners() {
		perNode[o]++
	}
	fmt.Println("shards por nó:")
	for _, a := range addrs {
		fmt.Printf("  %s → %d shards\n", a, perNode[a])
	}

	if err := sc.LoadSet("A", A); err != nil {
		fmt.Println("erro:", err)
		return
	}
	if err := sc.LoadSet("B", B); err != nil {
		fmt.Println("erro:", err)
		return
	}
	t = time.Now()
	distCnt, err := sc.IntersectStored("A", "B")
	dDist := time.Since(t)
	if err != nil {
		fmt.Println("erro:", err)
		return
	}
	fmt.Printf("\nLOCAL:        %10s → |A∩B| = %d\n", dLocal.Round(time.Microsecond), localCnt)
	fmt.Printf("RING+CLUSTER: %10s → |A∩B| = %d\n", dDist.Round(time.Microsecond), distCnt)
	if distCnt != uint64(localCnt) {
		fmt.Println("⚠️ DIVERGÊNCIA!")
	} else {
		fmt.Println("✅ idêntico ao local")
	}

	// rebalanceamento: entra um 4º nó
	ownersBefore := ownersFor(addrs, *shards, *replicas)
	addrs4 := append(append([]string(nil), addrs...), "127.0.0.1:19999")
	ownersAfter := ownersFor(addrs4, *shards, *replicas)
	moved := 0
	for s := range ownersBefore {
		if ownersBefore[s] != ownersAfter[s] {
			moved++
		}
	}
	fmt.Printf("\nrebalanceamento (entra +1 nó): %d/%d shards mudam de dono (%.0f%%, ideal ~%.0f%%)\n",
		moved, *shards, 100*float64(moved)/float64(*shards), 100/float64(*nnodes+1))
	fmt.Println("(no módulo ingênuo, ~todos os shards mudariam)")
}

func log2(x int) int {
	n := 0
	for x > 1 {
		x >>= 1
		n++
	}
	return n
}
