package cluster

import (
	"encoding/binary"
	"net"

	"github.com/setrixdb/setrixdb/internal/addb"
)

// ShardedCluster distribui S shards (faixas de palavras) entre nós, atribuindo
// cada shard a um nó via CONSISTENT HASH RING. Um nó pode possuir vários shards;
// seu armazenamento é a concatenação das faixas deles, por conjunto nomeado.
type ShardedCluster struct {
	W      int
	Shards int
	ring   *addb.Ring
	owners []string // shard -> endereço do nó
	conns  map[string]net.Conn
}

// NewShardedCluster monta o cluster: registra os nós no anel, atribui os shards
// e conecta. replicas = pontos virtuais por nó.
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

func (sc *ShardedCluster) wordsOfShard(s int) (int, int) {
	return s * sc.W / sc.Shards, (s + 1) * sc.W / sc.Shards
}

// nodeSlice concatena as faixas de palavras dos shards que pertencem a `node`.
// A MESMA ordem é usada no LoadSet e nas consultas → alinhamento garantido.
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

// LoadSet envia a cada nó a concatenação das faixas de `global` dos seus shards.
func (sc *ShardedCluster) LoadSet(name string, global []uint64) error {
	for node, conn := range sc.conns {
		slice := sc.nodeSlice(global, node)
		payload := []byte{opLoadSet}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(slice)...)
		if err := writeMsg(conn, payload); err != nil {
			return err
		}
		if _, err := readMsg(conn); err != nil { // ack
			return err
		}
	}
	return nil
}

// gather consulta todos os nós em paralelo e soma as contagens.
func (sc *ShardedCluster) gather(send func(node string) []byte) (uint64, error) {
	type res struct {
		n   uint64
		err error
	}
	ch := make(chan res, len(sc.conns))
	for node, conn := range sc.conns {
		go func(node string, conn net.Conn) {
			payload := send(node)
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

// IntersectStored calcula |A ∩ B| para dois conjuntos ARMAZENADOS (sem trafegar
// dados de consulta).
func (sc *ShardedCluster) IntersectStored(a, b string) (uint64, error) {
	return sc.gather(func(string) []byte {
		payload := []byte{opAndStored}
		payload = writeName(payload, a)
		payload = writeName(payload, b)
		return payload
	})
}

// IntersectQuery calcula |A ∩ Q| com Q ad-hoc (envia a fatia da consulta).
func (sc *ShardedCluster) IntersectQuery(name string, query []uint64) (uint64, error) {
	return sc.gather(func(node string) []byte {
		payload := []byte{opAndQuery}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(sc.nodeSlice(query, node))...)
		return payload
	})
}

// Close fecha as conexões.
func (sc *ShardedCluster) Close() {
	for _, c := range sc.conns {
		if c != nil {
			c.Close()
		}
	}
}
