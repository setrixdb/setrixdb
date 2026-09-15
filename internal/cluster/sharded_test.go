// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"math/bits"
	"math/rand"
	"testing"
)

func TestShardedCluster(t *testing.T) {
	W := 4096
	S := 7 // 7 shards p/ 3 nós → distribuição desigual
	A := make([]uint64, W)
	B := make([]uint64, W)
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 60000; i++ {
		A[r.Intn(W)] |= 1 << r.Intn(64)
	}
	for i := 0; i < 60000; i++ {
		B[r.Intn(W)] |= 1 << r.Intn(64)
	}
	var want uint64
	for i := 0; i < W; i++ {
		want += uint64(bits.OnesCount64(A[i] & B[i]))
	}

	var addrs []string
	var nodes []*Node
	for i := 0; i < 3; i++ {
		nd := NewNode("127.0.0.1:0")
		addr, err := nd.Listen()
		if err != nil {
			t.Fatal(err)
		}
		addrs = append(addrs, addr)
		nodes = append(nodes, nd)
	}
	defer func() {
		for _, nd := range nodes {
			nd.Close()
		}
	}()

	sc, err := NewShardedCluster(addrs, W, S, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()

	for s, o := range sc.Owners() {
		if o == "" {
			t.Fatalf("shard %d sem dono", s)
		}
	}
	if err := sc.LoadSet("A", A); err != nil {
		t.Fatal(err)
	}
	if err := sc.LoadSet("B", B); err != nil {
		t.Fatal(err)
	}
	if got, err := sc.IntersectQuery("A", B); err != nil || got != want {
		t.Fatalf("IntersectQuery: got=%d want=%d err=%v", got, want, err)
	}
	if got, err := sc.IntersectStored("A", "B"); err != nil || got != want {
		t.Fatalf("IntersectStored: got=%d want=%d err=%v", got, want, err)
	}
}
