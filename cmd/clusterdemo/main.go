// Command clusterdemo — demonstra o ADDB distribuído "over-the-wire": S nós
// servem shards do conjunto A por TCP; o coordenador faz broadcast da consulta B
// e agrega |A ∩ B|. Compara com a interseção local (1 máquina) e mede.
//
// Uso:
//
//	go run ./cmd/clusterdemo                 # S nós em loopback (127.0.0.1)
//	go run ./cmd/clusterdemo -nodes "ip:porta,..."   # nós remotos já escutando
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/tgosoul2019/addb/internal/cluster"
	"github.com/tgosoul2019/addb/internal/simd"
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
	nodesFlag := flag.String("nodes", "", "lista de endereços remotos (csv); vazio = nós locais")
	shards := flag.Int("shards", 4, "número de shards (só p/ nós locais)")
	universeBits := flag.Int("universe", 1<<26, "tamanho do universo em bits")
	queries := flag.Int("q", 200, "número de consultas a medir")
	flag.Parse()

	W := *universeBits / 64
	fmt.Printf("== ADDB distribuído (over-the-wire) — universo 2^%d, %d palavras, kernel %s ==\n\n",
		log2(*universeBits), W, simd.Name())

	// ---- nós ----
	var addrs []string
	var locals []*cluster.Node
	if *nodesFlag == "" {
		for i := 0; i < *shards; i++ {
			nd := cluster.NewNode(fmt.Sprintf("127.0.0.1:%d", 19100+i))
			addr, err := nd.Listen()
			if err != nil {
				fmt.Println("erro ao escutar:", err)
				return
			}
			addrs = append(addrs, addr)
			locals = append(locals, nd)
		}
		fmt.Printf("nós locais: %d (%s ...)\n", len(addrs), addrs[0])
		defer func() {
			for _, nd := range locals {
				nd.Close()
			}
		}()
	} else {
		addrs = strings.Split(*nodesFlag, ",")
		fmt.Printf("nós remotos: %d (%s)\n", len(addrs), addrs[0])
	}

	// ---- conjuntos A e B ----
	A := buildWords(W, 0.10, 1)  // A denso (10% dos bits)
	B := buildWords(W, 0.05, 7)  // B consulta (5%)
	fmt.Printf("conjuntos: |A| bits setados, |B| consulta — A=%d palavras\n\n", W)

	// ---- baseline local (1 máquina, kernel AVX-512) ----
	t := time.Now()
	localCnt := simd.AndPopcount(A, B)
	dLocal := time.Since(t)
	fmt.Printf("LOCAL (1 máquina, AVX-512):  %10s  → |A∩B| = %d\n", dLocal.Round(time.Microsecond), localCnt)

	// ---- distribuído ----
	c, err := cluster.Dial(addrs, W)
	if err != nil {
		fmt.Println("erro no dial:", err)
		return
	}
	defer c.Close()
	t = time.Now()
	if err := c.Load(A); err != nil {
		fmt.Println("erro no load:", err)
		return
	}
	dLoad := time.Since(t)

	t = time.Now()
	distCnt, err := c.IntersectCount(B)
	dFirst := time.Since(t)
	if err != nil {
		fmt.Println("erro na consulta:", err)
		return
	}
	fmt.Printf("DISTRIBUÍDO (%d nós):        %10s  → |A∩B| = %d  (load inicial %s)\n",
		len(addrs), dFirst.Round(time.Microsecond), distCnt, dLoad.Round(time.Millisecond))

	if distCnt != uint64(localCnt) {
		fmt.Printf("⚠️  DIVERGÊNCIA! local=%d dist=%d\n", localCnt, distCnt)
	} else {
		fmt.Printf("✅ resultados idênticos (local == distribuído)\n")
	}

	// ---- throughput distribuído (reusa um punhado de consultas p/ não estourar RAM) ----
	prep := 16
	if *queries < prep {
		prep = *queries
	}
	qs := make([][]uint64, prep)
	for i := range qs {
		qs[i] = buildWords(W, 0.05, int64(1000+i))
	}
	t = time.Now()
	var acc uint64
	for i := 0; i < *queries; i++ {
		n, err := c.IntersectCount(qs[i%prep])
		if err != nil {
			fmt.Println("erro:", err)
			return
		}
		acc += n
	}
	dAll := time.Since(t)
	perQ := dAll / time.Duration(*queries)
	fmt.Printf("\n%d consultas: %s total | %s/consulta | %.0f consultas/s (checksum=%d)\n",
		*queries, dAll.Round(time.Millisecond), perQ.Round(time.Microsecond), float64(*queries)/dAll.Seconds(), acc)
}

func log2(x int) int {
	n := 0
	for x > 1 {
		x >>= 1
		n++
	}
	return n
}
