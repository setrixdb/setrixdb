// Package cluster implementa a camada over-the-wire do ADDB: nós que servem um
// SHARD (fatia do bitset global) por TCP e um coordenador que faz broadcast da
// consulta e agrega. Protocolo binário mínimo (sem dependências).
//
// Modelo: o espaço de bits é particionado em S faixas de palavras. O nó i guarda
// as palavras [i·W/S, (i+1)·W/S). O coordenador envia a fatia correspondente da
// consulta; o nó faz AND-popcount local e devolve a contagem. A soma = |A ∩ B|.
package cluster

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"net"
	"sync"
	"unsafe"
)

const (
	opLoad  = 0 // [0][words...]  carrega o shard no nó
	opQuery = 1 // [1][words...]  → resposta [uint64 count]
	maxMsg  = 1 << 30
)

func writeMsg(w io.Writer, payload []byte) error {
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}

func readMsg(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if int(n) > maxMsg {
		return nil, fmt.Errorf("cluster: mensagem grande demais (%d)", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// bytesView devolve uma visão []byte sobre um []uint64 (zero-copy). Somente
// leitura; a ordem de bytes casa em little-endian (x86/arm64).
func bytesView(w []uint64) []byte {
	if len(w) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&w[0])), len(w)*8)
}

// wordsView devolve uma visão []uint64 sobre um []byte (zero-copy).
func wordsView(b []byte) []uint64 {
	if len(b) < 8 {
		return nil
	}
	return unsafe.Slice((*uint64)(unsafe.Pointer(&b[0])), len(b)/8)
}

// Node serve um shard por TCP.
type Node struct {
	Addr  string
	mu    sync.RWMutex
	words []uint64
	ln    net.Listener
}

// NewNode cria um nó (ainda sem escutar).
func NewNode(addr string) *Node { return &Node{Addr: addr} }

// Listen sobe o servidor e devolve o endereço efetivo.
func (n *Node) Listen() (string, error) {
	ln, err := net.Listen("tcp", n.Addr)
	if err != nil {
		return "", err
	}
	n.ln = ln
	go n.serve()
	return ln.Addr().String(), nil
}

func (n *Node) serve() {
	for {
		c, err := n.ln.Accept()
		if err != nil {
			return
		}
		go n.handle(c)
	}
}

func (n *Node) handle(c net.Conn) {
	defer c.Close()
	for {
		msg, err := readMsg(c)
		if err != nil {
			return
		}
		if len(msg) < 1 {
			continue
		}
		switch msg[0] {
		case opLoad:
			shard := append([]uint64(nil), wordsView(msg[1:])...)
			n.mu.Lock()
			n.words = shard
			n.mu.Unlock()
		case opQuery:
			q := wordsView(msg[1:])
			n.mu.RLock()
			var cnt uint64
			m := len(q)
			if len(n.words) < m {
				m = len(n.words)
			}
			for i := 0; i < m; i++ {
				cnt += uint64(bits.OnesCount64(q[i] & n.words[i]))
			}
			n.mu.RUnlock()
			var resp [8]byte
			binary.LittleEndian.PutUint64(resp[:], cnt)
			if err := writeMsg(c, resp[:]); err != nil {
				return
			}
		default:
			return
		}
	}
}

// Close encerra o servidor.
func (n *Node) Close() {
	if n.ln != nil {
		n.ln.Close()
	}
}

// Coordinator fala com S nós e agrega as contagens.
type Coordinator struct {
	conns []net.Conn
	lo    []int
	hi    []int
}

// Dial conecta aos nós. W = total de palavras do bitset global; as faixas são
// particionadas igualmente entre os nós (na mesma ordem dos endereços).
func Dial(addrs []string, W int) (*Coordinator, error) {
	S := len(addrs)
	c := &Coordinator{}
	for i, a := range addrs {
		conn, err := net.Dial("tcp", a)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.conns = append(c.conns, conn)
		c.lo = append(c.lo, i*W/S)
		c.hi = append(c.hi, (i+1)*W/S)
	}
	return c, nil
}

// Load distribui as fatias do conjunto A (shard global) para cada nó.
func (c *Coordinator) Load(globalA []uint64) error {
	for i := range c.conns {
		words := globalA[c.lo[i]:c.hi[i]]
		payload := make([]byte, 1+len(words)*8)
		payload[0] = opLoad
		copy(payload[1:], bytesView(words))
		if err := writeMsg(c.conns[i], payload); err != nil {
			return err
		}
	}
	return nil
}

// IntersectCount envia a fatia da consulta B para cada nó e soma |A_shard ∩ B|.
// O broadcast é PARALELO entre os nós (cada nó tem sua conexão).
func (c *Coordinator) IntersectCount(query []uint64) (uint64, error) {
	counts := make([]uint64, len(c.conns))
	errs := make([]error, len(c.conns))
	var wg sync.WaitGroup
	for i := range c.conns {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			words := query[c.lo[i]:c.hi[i]]
			payload := make([]byte, 1+len(words)*8)
			payload[0] = opQuery
			copy(payload[1:], bytesView(words))
			if err := writeMsg(c.conns[i], payload); err != nil {
				errs[i] = err
				return
			}
			resp, err := readMsg(c.conns[i])
			if err != nil {
				errs[i] = err
				return
			}
			counts[i] = binary.LittleEndian.Uint64(resp)
		}(i)
	}
	wg.Wait()
	var total uint64
	for i := range counts {
		if errs[i] != nil {
			return 0, errs[i]
		}
		total += counts[i]
	}
	return total, nil
}

// Close fecha todas as conexões.
func (c *Coordinator) Close() {
	for _, cn := range c.conns {
		if cn != nil {
			cn.Close()
		}
	}
}
