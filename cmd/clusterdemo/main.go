// Command clusterdemo — demonstra o SetrixDB distribuído e COMPARA os dois modos de
// interseção:
//
//	ARMazenado: |A ∩ B| com A,B já armazenados → cada nó faz o AND local; a rede
//	            só carrega o NOME dos conjuntos (mensagens minúsculas).
//	AD-HOC:     |A ∩ Q| com Q ad-hoc → o coordenador transmite a fatia de Q.
//
// Uso:
//
//	go run ./cmd/clusterdemo                       # nós locais
//	go run ./cmd/clusterdemo -nodes "ip:p,..."     # nós remotos
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

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

func main() {
	nodesFlag := flag.String("nodes", "", "endereços remotos (csv); vazio = locais")
	shards := flag.Int("shards", 4, "nós locais")
	universe := flag.Int("universe", 1<<26, "universo em bits")
	reps := flag.Int("q", 500, "repetições por modo")
	flag.Parse()

	W := *universe / 64
	fmt.Printf("== SetrixDB distribuído — armazenado vs ad-hoc · universo 2^%d, %d palavras ==\n\n", log2(*universe), W)

	var addrs []string
	var locals []*cluster.Node
	if *nodesFlag == "" {
		for i := 0; i < *shards; i++ {
			nd := cluster.NewNode(fmt.Sprintf("127.0.0.1:%d", 19100+i))
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
	} else {
		addrs = strings.Split(*nodesFlag, ",")
	}
	fmt.Printf("nós: %d (%s)\n", len(addrs), addrs[0])

	A := buildWords(W, 0.10, 1)
	B := buildWords(W, 0.05, 7)
	local := simd.AndPopcount(A, B)

	c, err := cluster.Dial(addrs, W)
	if err != nil {
		fmt.Println("erro:", err)
		return
	}
	defer c.Close()

	fmt.Println("\ncarregando A e B (uma vez)...")
	t := time.Now()
	if err := c.LoadSet("A", A); err != nil {
		fmt.Println("erro:", err)
		return
	}
	if err := c.LoadSet("B", B); err != nil {
		fmt.Println("erro:", err)
		return
	}
	fmt.Printf("load: %s\n", time.Since(t).Round(time.Millisecond))

	// correção (ambos os modos)
	fmt.Printf("\nLOCAL: |A∩B| = %d\n", local)
	gs, err := c.IntersectStored("A", "B")
	if err != nil {
		fmt.Println("erro:", err)
		return
	}
	gq, err := c.IntersectQuery("A", B)
	if err != nil {
		fmt.Println("erro:", err)
		return
	}
	fmt.Printf("DISTRIB armazenado: %d  %s\n", gs, ok(gs == uint64(local)))
	fmt.Printf("DISTRIB ad-hoc:     %d  %s\n", gq, ok(gq == uint64(local)))

	// latência: armazenado (sem tráfego de dados)
	t = time.Now()
	for i := 0; i < *reps; i++ {
		if _, err := c.IntersectStored("A", "B"); err != nil {
			fmt.Println("erro:", err)
			return
		}
	}
	dStored := time.Since(t) / time.Duration(*reps)

	// latência: ad-hoc (transmite a fatia de B a cada consulta)
	qs := make([][]uint64, 8)
	for i := range qs {
		qs[i] = buildWords(W, 0.05, int64(100+i))
	}
	t = time.Now()
	for i := 0; i < *reps; i++ {
		if _, err := c.IntersectQuery("A", qs[i%len(qs)]); err != nil {
			fmt.Println("erro:", err)
			return
		}
	}
	dQuery := time.Since(t) / time.Duration(*reps)

	fmt.Printf("\n-- latência por consulta (%d rep) --\n", *reps)
	fmt.Printf("  armazenado (só nomes):  %10s   ← sem tráfego de dados\n", dStored.Round(time.Microsecond))
	fmt.Printf("  ad-hoc (transmite B):   %10s\n", dQuery.Round(time.Microsecond))
	if dQuery > 0 {
		fmt.Printf("  → armazenado é %.0f× mais rápido\n", float64(dQuery)/float64(dStored))
	}
}

func ok(b bool) string {
	if b {
		return "✅"
	}
	return "⚠️"
}

func log2(x int) int {
	n := 0
	for x > 1 {
		x >>= 1
		n++
	}
	return n
}
