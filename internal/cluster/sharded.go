package cluster

import (
	"encoding/binary"
	"net"

	"github.com/tgosoul2019/addb/internal/addb"
)

// ShardedCluster distribui S shards (faixas de palavras do espaço global) entre
// nós, atribuindo cada shard a um nó via CONSISTENT HASH RING. Um nó pode
// possuir vários shards; seu armazenamento é a concatenação das faixas deles.
//
// Diferença do Coordinator simples: lá a partição é fixa (em ordem de endereço);
// aqui a pertinência é do anel → entrar/sair nó remapeia só ~1/(N+1) dos shards.
type ShardedCluster struct {
	W      int
	Shards int
	ring   *addb.Ring
	owners []string // shard -> endereço do nó
	conns  map[string]net.Conn
}

// NewShardedCluster monta o cluster: registra os nós no anel, atribui os shards e
// conecta. replicas = pontos virtuais por nó.
func NewShardedCluster(addrs []string, W, shards, replicas int) (*ShardedCluster, error) {
	ring := addb.NewRing(replicas)
	for _, a := range addrs {
		ring.AddNode(a)
	}
	sc := &ShardedCluster{W: W, Shards: shards, ring: ring, conns: map[string]net.Conn{}}
	sc.owners = make([]string, shards)
	for s := 0; s < shards; s++ {
		sc.owners[s], _ = ring.Route(uint64(s))
	}
	for _, a := range addrs {
		c, err := net.Dial("tcp", a)
		if err != nil {
			sc.Close()
			return nil, err
		}
		sc.conns[a] = c
	}
	return sc, nil
}

// Owners devolve o dono (endereço) de cada shard.
func (sc *ShardedCluster) Owners() []string { return append([]string(nil), sc.owners...) }

// wordsOfShard devolve a faixa [lo,hi) de palavras do shard s.
func (sc *ShardedCluster) wordsOfShard(s int) (int, int) {
	lo := s * sc.W / sc.Shards
	hi := (s + 1) * sc.W / sc.Shards
	return lo, hi
}

// nodeSlice concatena as faixas de palavras dos shards que pertencem a `node`.
// A MESMA ordem é usada no Load e na consulta → alinhamento garantido.
func (sc *ShardedCluster) nodeSlice(global []uint64, node string) []uint64 {
	var buf []uint64
	for s := 0; s < sc.Shards; s++ {
		if sc.owners[s] != node {
			continue
		}
		lo, hi := sc.wordsOfShard(s)
		buf = append(buf, global[lo:hi]...)
	}
	return buf
}

// Load envia a cada nó a concatenação das faixas de A dos seus shards.
func (sc *ShardedCluster) Load(A []uint64) error {
	for node, conn := range sc.conns {
		slice := sc.nodeSlice(A, node)
		payload := make([]byte, 1+len(slice)*8)
		payload[0] = opLoad
		copy(payload[1:], bytesView(slice))
		if err := writeMsg(conn, payload); err != nil {
			return err
		}
	}
	return nil
}

// IntersectCount consulta cada nó (paralelo) com a fatia dos seus shards e soma.
func (sc *ShardedCluster) IntersectCount(B []uint64) (uint64, error) {
	type res struct {
		n   uint64
		err error
	}
	ch := make(chan res, len(sc.conns))
	for node, conn := range sc.conns {
		go func(node string, conn net.Conn) {
			slice := sc.nodeSlice(B, node)
			payload := make([]byte, 1+len(slice)*8)
			payload[0] = opQuery
			copy(payload[1:], bytesView(slice))
			if err := writeMsg(conn, payload); err != nil {
				ch <- res{0, err}
				return
			}
			resp, err := readMsg(conn)
			if err != nil {
				ch <- res{0, err}
				return
			}
			ch <- res{binary.LittleEndian.Uint64(resp), nil}
		}(node, conn)
	}
	var total uint64
	for range sc.conns {
		r := <-ch
		if r.err != nil {
			return 0, r.err
		}
		total += r.n
	}
	return total, nil
}

// Close fecha as conexões.
func (sc *ShardedCluster) Close() {
	for _, c := range sc.conns {
		if c != nil {
			c.Close()
		}
	}
}
