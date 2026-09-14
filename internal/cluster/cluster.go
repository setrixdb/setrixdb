// Package cluster implementa a camada over-the-wire do ADDB: nós que servem
// SHARDS (fatias do bitset global) por TCP e um coordenador que agrega.
// Protocolo binário mínimo, zero-copy (sem dependências).
//
// Modelo de dados: um nó guarda CONJUNTOS nomeados ("sets"), cada um a
// concatenação das faixas de palavras dos shards que ele possui. Dois modos de
// interseção:
//
//	AND entre dois conjuntos ARMAZENADOS (a,b): nada trafega além do nome dos
//	conjuntos → cada nó faz o AND local. É o modo escalável.
//
//	AND contra uma consulta AD-HOC: o coordenador envia a fatia da consulta.
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
	opLoadSet   = 0 // [0][u16 nLen][name][words...]
	opAndStored = 1 // [1][u16 aLen][a][u16 bLen][b]        -> [u64 count]
	opAndQuery  = 2 // [2][u16 nLen][name][words...]        -> [u64 count]
	maxMsg      = 1 << 30
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

func bytesView(w []uint64) []byte {
	if len(w) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&w[0])), len(w)*8)
}

func wordsView(b []byte) []uint64 {
	if len(b) < 8 {
		return nil
	}
	return unsafe.Slice((*uint64)(unsafe.Pointer(&b[0])), len(b)/8)
}

// writeName acrescenta [u16 len][nome] ao buffer.
func writeName(buf []byte, name string) []byte {
	var l [2]byte
	binary.LittleEndian.PutUint16(l[:], uint16(len(name)))
	buf = append(buf, l[:]...)
	return append(buf, name...)
}

// readName lê [u16 len][nome] de b e devolve (nome, resto, ok).
func readName(b []byte) (string, []byte, bool) {
	if len(b) < 2 {
		return "", nil, false
	}
	n := int(binary.LittleEndian.Uint16(b[:2]))
	if len(b) < 2+n {
		return "", nil, false
	}
	return string(b[2 : 2+n]), b[2+n:], true
}

func andCount(a, b []uint64) uint64 {
	m := len(a)
	if len(b) < m {
		m = len(b)
	}
	var c uint64
	for i := 0; i < m; i++ {
		c += uint64(bits.OnesCount64(a[i] & b[i]))
	}
	return c
}

// Node serve shards/conjuntos por TCP.
type Node struct {
	Addr string
	mu   sync.RWMutex
	sets map[string][]uint64
	ln   net.Listener
}

func NewNode(addr string) *Node { return &Node{Addr: addr, sets: map[string][]uint64{}} }

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
		var count uint64
		switch msg[0] {
		case opLoadSet:
			name, rest, ok := readName(msg[1:])
			if !ok {
				continue
			}
			shard := append([]uint64(nil), wordsView(rest)...)
			n.mu.Lock()
			n.sets[name] = shard
			n.mu.Unlock()
			// sem resposta (fire-and-forget) — mas completa para sincronia:
			writeMsg(c, make([]byte, 8))
			continue
		case opAndStored:
			a, rest, ok := readName(msg[1:])
			if !ok {
				continue
			}
			b, _, ok := readName(rest)
			if !ok {
				continue
			}
			n.mu.RLock()
			count = andCount(n.sets[a], n.sets[b])
			n.mu.RUnlock()
		case opAndQuery:
			name, rest, ok := readName(msg[1:])
			if !ok {
				continue
			}
			q := wordsView(rest)
			n.mu.RLock()
			count = andCount(n.sets[name], q)
			n.mu.RUnlock()
		default:
			return
		}
		var resp [8]byte
		binary.LittleEndian.PutUint64(resp[:], count)
		if err := writeMsg(c, resp[:]); err != nil {
			return
		}
	}
}

func (n *Node) Close() {
	if n.ln != nil {
		n.ln.Close()
	}
}

// Coordinator fala com S nós (partição fixa por faixa de palavras).
type Coordinator struct {
	conns []net.Conn
	lo    []int
	hi    []int
}

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

// LoadSet envia a cada nó a fatia (do seu intervalo) do conjunto `global`.
func (c *Coordinator) LoadSet(name string, global []uint64) error {
	for i := range c.conns {
		payload := []byte{opLoadSet}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(global[c.lo[i]:c.hi[i]])...)
		if err := writeMsg(c.conns[i], payload); err != nil {
			return err
		}
		if _, err := readMsg(c.conns[i]); err != nil { // ack
			return err
		}
	}
	return nil
}

func (c *Coordinator) gather(send func(i int) ([]byte, error)) (uint64, error) {
	counts := make([]uint64, len(c.conns))
	errs := make([]error, len(c.conns))
	var wg sync.WaitGroup
	for i := range c.conns {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload, err := send(i)
			if err != nil {
				errs[i] = err
				return
			}
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

// IntersectStored calcula |A ∩ B| p/ dois conjuntos ARMAZENADOS — sem trafegar
// dados de consulta (só os nomes).
func (c *Coordinator) IntersectStored(a, b string) (uint64, error) {
	return c.gather(func(i int) ([]byte, error) {
		payload := []byte{opAndStored}
		payload = writeName(payload, a)
		payload = writeName(payload, b)
		return payload, nil
	})
}

// IntersectQuery calcula |A ∩ Q| onde A é armazenado e Q é uma consulta ad-hoc
// (envia a fatia da consulta para cada nó).
func (c *Coordinator) IntersectQuery(name string, query []uint64) (uint64, error) {
	return c.gather(func(i int) ([]byte, error) {
		payload := []byte{opAndQuery}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(query[c.lo[i]:c.hi[i]])...)
		return payload, nil
	})
}

func (c *Coordinator) Close() {
	for _, cn := range c.conns {
		if cn != nil {
			cn.Close()
		}
	}
}
