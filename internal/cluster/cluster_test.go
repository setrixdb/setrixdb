package cluster

import (
	"math/bits"
	"math/rand"
	"testing"
)

func TestClusterIntersect(t *testing.T) {
	W := 4096
	A := make([]uint64, W)
	B := make([]uint64, W)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 50000; i++ {
		A[r.Intn(W)] |= 1 << r.Intn(64)
	}
	for i := 0; i < 50000; i++ {
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

	c, err := Dial(addrs, W)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.LoadSet("A", A); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadSet("B", B); err != nil {
		t.Fatal(err)
	}

	// consulta ad-hoc (envia a fatia de B)
	if got, err := c.IntersectQuery("A", B); err != nil || got != want {
		t.Fatalf("IntersectQuery: got=%d want=%d err=%v", got, want, err)
	}
	// conjuntos armazenados (não envia dados)
	if got, err := c.IntersectStored("A", "B"); err != nil || got != want {
		t.Fatalf("IntersectStored: got=%d want=%d err=%v", got, want, err)
	}
}
