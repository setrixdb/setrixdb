// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"encoding/binary"
	"errors"
	"fmt"

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
	peers  map[string]*PeerConn
	opt    Options
}

// NewShardedCluster monta o cluster: registra os nós no anel, atribui os shards
// e conecta. replicas = pontos virtuais por nó.
func NewShardedCluster(addrs []string, W, shards, replicas int) (*ShardedCluster, error) {
	return NewShardedClusterWithOptions(addrs, W, shards, replicas, DefaultOptions())
}

// NewShardedClusterWithOptions — como NewShardedCluster, com limites explícitos
// de dial/IO/reconexão.
func NewShardedClusterWithOptions(addrs []string, W, shards, replicas int, opt Options) (*ShardedCluster, error) {
	if len(addrs) == 0 {
		return nil, errors.New("cluster: nenhum endereço de nó")
	}
	if shards <= 0 {
		return nil, fmt.Errorf("cluster: shards deve ser > 0 (%d)", shards)
	}
	opt = opt.withDefaults()
	ring := addb.NewRing(replicas)
	for _, a := range addrs {
		ring.AddNode(a)
	}
	sc := &ShardedCluster{W: W, Shards: shards, ring: ring, peers: map[string]*PeerConn{}, opt: opt}
	sc.owners = make([]string, shards)
	for s := 0; s < shards; s++ {
		sc.owners[s], _ = ring.Route(uint64(s))
	}
	for _, a := range addrs {
		p := newPeerConn(a, opt)
		if err := p.Connect(); err != nil {
			sc.Close()
			return nil, err
		}
		sc.peers[a] = p
	}
	return sc, nil
}

// Health — endereço → está fora? (última operação falhou).
func (sc *ShardedCluster) Health() map[string]bool {
	out := make(map[string]bool, len(sc.peers))
	for a, p := range sc.peers {
		out[a] = p.Down()
	}
	return out
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
	if err := validateName(name); err != nil {
		return err
	}
	for node, peer := range sc.peers {
		slice := sc.nodeSlice(global, node)
		payload := []byte{opLoadSet}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(slice)...)
		if _, err := peer.roundTrip(payload); err != nil { // inclui o ack
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
	ch := make(chan res, len(sc.peers))
	for node, peer := range sc.peers {
		go func(node string, peer *PeerConn) {
			payload := send(node)
			resp, err := peer.roundTrip(payload)
			if err != nil {
				ch <- res{0, err}
				return
			}
			if len(resp) < 8 {
				ch <- res{0, fmt.Errorf("cluster: resposta curta (%d bytes) de %s", len(resp), peer.Addr())}
				return
			}
			ch <- res{binary.LittleEndian.Uint64(resp), nil}
		}(node, peer)
	}
	var total uint64
	var firstErr error
	for range sc.peers {
		r := <-ch
		if r.err != nil {
			// Não aborta na primeira falha: as outras goroutines precisam
			// drenar o canal (senão vazam com o canal cheio).
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		total += r.n
	}
	if firstErr != nil {
		return 0, firstErr
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
	for _, p := range sc.peers {
		if p != nil {
			p.Close()
		}
	}
}
