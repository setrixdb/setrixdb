// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// opts rápidas: o teste não pode esperar 3 s + 10 s por falha.
func testOpts() Options {
	return Options{DialTimeout: 300 * time.Millisecond, IOTimeout: 500 * time.Millisecond, Retries: 1, Backoff: 20 * time.Millisecond}
}

func startNodes(t *testing.T, k int) ([]*Node, []string) {
	t.Helper()
	nodes := make([]*Node, k)
	addrs := make([]string, k)
	for i := 0; i < k; i++ {
		nd := NewNode("127.0.0.1:0")
		addr, err := nd.Listen()
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		nodes[i], addrs[i] = nd, addr
	}
	return nodes, addrs
}

func stopNodes(nodes []*Node) {
	for _, nd := range nodes {
		if nd != nil {
			nd.Close()
		}
	}
}

func randomWords(n int, seed int64) []uint64 {
	r := rand.New(rand.NewSource(seed))
	w := make([]uint64, n)
	for i := 0; i < n*8; i++ {
		w[r.Intn(n)] |= 1 << r.Intn(64)
	}
	return w
}

// Nó morrendo NO MEIO da query: erro é ErrPeerDown, chega dentro do limite e o
// peer aparece como fora no Health — nada de operação pendurada.
func TestNodeDiesMidQuery(t *testing.T) {
	nodes, addrs := startNodes(t, 3)
	defer stopNodes(nodes)

	W := 1024
	A := randomWords(W, 1)
	c, err := DialWithOptions(addrs, W, testOpts())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.LoadSet("A", A); err != nil {
		t.Fatalf("loadset: %v", err)
	}
	if _, err := c.IntersectQuery("A", A); err != nil {
		t.Fatalf("consulta saudável falhou: %v", err)
	}

	// mata um nó (fecha listener + derruba conexões)
	nodes[1].Close()

	start := time.Now()
	_, err = c.IntersectQuery("A", A)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("query com nó morto deveria falhar")
	}
	if !errors.Is(err, ErrPeerDown) {
		t.Fatalf("erro deveria ser ErrPeerDown, veio: %v", err)
	}
	if !strings.Contains(err.Error(), addrs[1]) {
		t.Fatalf("erro deveria nomear o peer %s: %v", addrs[1], err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("demorou %v — deadline não está valendo", elapsed)
	}

	h := c.Health()
	if !h[addrs[1]] {
		t.Fatalf("Health deveria marcar %s como fora: %v", addrs[1], h)
	}
	// os outros continuam de pé no relatório
	for i, a := range addrs {
		if i != 1 && h[a] {
			t.Fatalf("peer %s não deveria estar marcado como fora", a)
		}
	}
}

// Nó volta no MESMO endereço: a próxima operação reconecta e a resposta vem
// certa (o dado é recarregado porque o nó é novo).
func TestReconnectAfterNodeRestart(t *testing.T) {
	nodes, addrs := startNodes(t, 2)
	defer func() { stopNodes(nodes) }()

	W := 512
	A := randomWords(W, 2)
	B := randomWords(W, 3)
	c, err := DialWithOptions(addrs, W, testOpts())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.LoadSet("A", A); err != nil {
		t.Fatalf("loadset A: %v", err)
	}
	if err := c.LoadSet("B", B); err != nil {
		t.Fatalf("loadset B: %v", err)
	}

	nodes[0].Close()
	if _, err := c.IntersectStored("A", "B"); err == nil || !errors.Is(err, ErrPeerDown) {
		t.Fatalf("esperava ErrPeerDown com nó morto, veio %v", err)
	}

	// sobe de novo no mesmo endereço
	nd := NewNode(addrs[0])
	if _, err := nd.Listen(); err != nil {
		t.Fatalf("relisten no mesmo endereço: %v", err)
	}
	defer nd.Close()
	nodes[0] = nd

	// recarrega os conjuntos (nó novo começa vazio) e consulta: reconectou
	if err := c.LoadSet("A", A); err != nil {
		t.Fatalf("loadset A pós-restart: %v", err)
	}
	if err := c.LoadSet("B", B); err != nil {
		t.Fatalf("loadset B pós-restart: %v", err)
	}
	if _, err := c.IntersectStored("A", "B"); err != nil {
		t.Fatalf("consulta pós-restart deveria funcionar: %v", err)
	}
	if h := c.Health(); h[addrs[0]] {
		t.Fatalf("peer reconectado não pode seguir marcado como fora: %v", h)
	}
}

// Dial em endereço que não responde respeita o DialTimeout (não pendura).
func TestDialTimeout(t *testing.T) {
	start := time.Now()
	_, err := DialWithOptions([]string{"192.0.2.1:9"}, 64, Options{DialTimeout: 200 * time.Millisecond, IOTimeout: 200 * time.Millisecond, Retries: 0})
	if err == nil {
		t.Fatal("dial para endereço morto deveria falhar")
	}
	if !errors.Is(err, ErrPeerDown) {
		t.Fatalf("erro deveria ser ErrPeerDown: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("dial levou %v — timeout não está valendo", elapsed)
	}
}

// Nome de conjunto fora do formato do protocolo é recusado ANTES de ir para a
// rede (antes, >65535 bytes era truncado em silêncio).
func TestNameValidation(t *testing.T) {
	nodes, addrs := startNodes(t, 1)
	defer stopNodes(nodes)
	c, err := DialWithOptions(addrs, 64, testOpts())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if err := c.LoadSet("", make([]uint64, 64)); err == nil {
		t.Fatal("nome vazio deveria ser recusado")
	}
	long := strings.Repeat("a", maxNameLen+1)
	if err := c.LoadSet(long, make([]uint64, 64)); err == nil {
		t.Fatal("nome maior que o formato deveria ser recusado")
	}
	if _, err := c.IntersectStored("", "x"); err == nil {
		t.Fatal("IntersectStored com nome vazio deveria ser recusado")
	}
}

// Consulta menor que a faixa dos nós é erro de cliente, não panic.
func TestQueryTooShort(t *testing.T) {
	nodes, addrs := startNodes(t, 2)
	defer stopNodes(nodes)
	c, err := DialWithOptions(addrs, 256, testOpts())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if _, err := c.IntersectQuery("A", make([]uint64, 10)); err == nil {
		t.Fatal("consulta curta deveria ser recusada")
	}
}

// ShardedCluster: um nó cai → erro é ErrPeerDown, os demais drenam e o
// cluster não fica pendurado.
func TestShardedNodeDies(t *testing.T) {
	nodes, addrs := startNodes(t, 3)
	defer stopNodes(nodes)

	W := 512
	A := randomWords(W, 5)
	sc, err := NewShardedClusterWithOptions(addrs, W, 6, 200, testOpts())
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	defer sc.Close()
	if err := sc.LoadSet("A", A); err != nil {
		t.Fatalf("loadset: %v", err)
	}
	if _, err := sc.IntersectQuery("A", A); err != nil {
		t.Fatalf("consulta saudável falhou: %v", err)
	}

	nodes[2].Close()
	start := time.Now()
	_, err = sc.IntersectQuery("A", A)
	if err == nil || !errors.Is(err, ErrPeerDown) {
		t.Fatalf("esperava ErrPeerDown, veio %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("sharded demorou %v", elapsed)
	}
	if h := sc.Health(); !h[addrs[2]] {
		t.Fatalf("Health do sharded deveria marcar %s: %v", addrs[2], h)
	}
}
