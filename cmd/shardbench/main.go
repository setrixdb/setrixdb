// Command shardbench — mede a escala horizontal do SetrixDB: particiona o espaço
// de IDs em N shards (ranges contíguos, cada um um bitset denso) e compara a
// interseção serial vs. paralela.
//
// Uso: go run ./cmd/shardbench
package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func main() {
	const universe = uint64(1) << 32 // 4.29 bilhões de IDs
	const keys = 4_000_000

	fmt.Printf("== SetrixDB sharding — universo 2^32, keys=%d, %d CPUs ==\n\n",
		keys, runtime.GOMAXPROCS(0))
	fmt.Printf("%-8s %12s %12s %12s\n", "shards", "mem total", "intersec ser", "intersec par")
	fmt.Println("--------------------------------------------------------")

	for _, S := range []int{4, 16, 64, 256} {
		A := addb.NewShardedBitset(universe, S)
		B := addb.NewShardedBitset(universe, S)

		// A: keys ids aleatórios (universo inteiro). B: 50% de A + 50% novos.
		ra := rand.New(rand.NewSource(12345 + int64(S)))
		var over []uint64
		for i := 0; i < keys; i++ {
			id := uint64(ra.Int63n(int64(universe)))
			A.Set(id)
			if i%2 == 0 {
				over = append(over, id)
			}
		}
		rb := rand.New(rand.NewSource(999 + int64(S)))
		for _, id := range over {
			B.Set(id)
		}
		for i := 0; i < keys/2; i++ {
			B.Set(uint64(rb.Int63n(int64(universe))))
		}

		t := time.Now()
		ser := A.IntersectCountSerial(B)
		dSer := time.Since(t)
		t = time.Now()
		par := A.IntersectCount(B)
		dPar := time.Since(t)

		fmt.Printf("%-8d %12.1f MB %12s %12s   (res=%d)\n",
			S, float64(A.MemBytes())/1e6, dSer.Round(time.Millisecond), dPar.Round(time.Millisecond), ser)
		_ = par
	}
}
