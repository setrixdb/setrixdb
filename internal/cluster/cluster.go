// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Package cluster implementa a camada over-the-wire do SetrixDB: nós que servem
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
	"errors"
	"fmt"
	"io"
	"math/bits"
	"net"
	"sync"
	"time"
	"unsafe"
)

const (
	opLoadSet   = 0 // [0][u16 nLen][name][words...]
	opAndStored = 1 // [1][u16 aLen][a][u16 bLen][b]        -> [u64 count]
	opAndQuery  = 2 // [2][u16 nLen][name][words...]        -> [u64 count]
	maxMsg      = 1 << 30
	maxNameLen  = 1<<16 - 1 // o nome vai em [u16 len]
)

// ErrPeerDown — o nó não respondeu depois das tentativas de reconexão. É
// sentinela: quem chama distingue "peer caiu" de erro de protocolo/dados.
var ErrPeerDown = errors.New("cluster: peer indisponível")

// Options — limites de robustez do cliente (coordenador/sharded).
type Options struct {
	DialTimeout time.Duration // conectar a um nó
	IOTimeout   time.Duration // escrever + ler uma resposta
	Retries     int           // tentativas extras de reconexão por operação
	Backoff     time.Duration // espera entre tentativas
}

// DefaultOptions — 3 s para conectar, 10 s de IO, 2 reconexões.
func DefaultOptions() Options {
	return Options{DialTimeout: 3 * time.Second, IOTimeout: 10 * time.Second, Retries: 2, Backoff: 100 * time.Millisecond}
}

func (o Options) withDefaults() Options {
	if o.DialTimeout <= 0 {
		o.DialTimeout = 3 * time.Second
	}
	if o.IOTimeout <= 0 {
		o.IOTimeout = 10 * time.Second
	}
	if o.Retries < 0 {
		o.Retries = 0
	}
	if o.Backoff <= 0 {
		o.Backoff = 100 * time.Millisecond
	}
	return o
}

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

// validateName confere o limite do formato (o nome viaja em [u16 len]) e o
// tamanho mínimo. Antes, um nome > 65535 era truncado em silêncio e o nó
// guardava outro conjunto.
func validateName(name string) error {
	if name == "" {
		return errors.New("cluster: nome de conjunto vazio")
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("cluster: nome de conjunto com %d bytes (máximo %d)", len(name), maxNameLen)
	}
	return nil
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

// PeerConn — conexão com um nó com deadline, reconexão e estado.
//
// Antes, uma conexão morta deixava a operação pendurada para sempre (read sem
// deadline) e o coordenador só sabia dizer "erro de IO". Agora: deadline por
// operação, reconexão com backoff e erro sentinela ErrPeerDown quando o nó não
// volta. As três operações do protocolo são idempotentes (loadSet sobrescreve,
// AND é leitura), então reenviar depois de reconectar é seguro.
type PeerConn struct {
	addr string
	opt  Options

	mu   sync.Mutex
	conn net.Conn
	down bool
}

func newPeerConn(addr string, opt Options) *PeerConn { return &PeerConn{addr: addr, opt: opt} }

// Addr — endereço do nó.
func (p *PeerConn) Addr() string { return p.addr }

// Down — a última operação falhou (o nó pode estar fora).
func (p *PeerConn) Down() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.down
}

// ensure conecta se preciso. Chamar com p.mu travado.
func (p *PeerConn) ensure() error {
	if p.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", p.addr, p.opt.DialTimeout)
	if err != nil {
		p.down = true
		return fmt.Errorf("%w: %s: %v", ErrPeerDown, p.addr, err)
	}
	p.conn = conn
	p.down = false
	return nil
}

func (p *PeerConn) closeLocked() {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// roundTrip envia o payload e devolve a resposta. Reconecta e repete até
// Options.Retries; se não voltar, devolve ErrPeerDown.
func (p *PeerConn) roundTrip(payload []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt <= p.opt.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(p.opt.Backoff)
			p.closeLocked()
		}
		if err := p.ensure(); err != nil {
			lastErr = err
			continue
		}
		_ = p.conn.SetDeadline(time.Now().Add(p.opt.IOTimeout))
		if err := writeMsg(p.conn, payload); err != nil {
			lastErr = err
			p.down = true
			p.closeLocked()
			continue
		}
		resp, err := readMsg(p.conn)
		if err != nil {
			lastErr = err
			p.down = true
			p.closeLocked()
			continue
		}
		_ = p.conn.SetDeadline(time.Time{})
		return resp, nil
	}
	p.down = true
	return nil, fmt.Errorf("%w: %s: %v", ErrPeerDown, p.addr, lastErr)
}

// Connect garante a conexão (usado no Dial e por quem precisa de handshake).
func (p *PeerConn) Connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ensure()
}

// Close fecha a conexão (o nó pode ser reconectado depois).
func (p *PeerConn) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

// Node serve shards/conjuntos por TCP.
type Node struct {
	Addr string
	// IdleTimeout fecha conexão ociosa (0 = sem limite). Um cliente que morre
	// sem RST deixa de ocupar goroutine para sempre.
	IdleTimeout time.Duration

	mu     sync.RWMutex
	sets   map[string][]uint64
	ln     net.Listener
	cMu    sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
}

func NewNode(addr string) *Node {
	return &Node{Addr: addr, IdleTimeout: 10 * time.Minute, sets: map[string][]uint64{}, conns: map[net.Conn]struct{}{}}
}

func (n *Node) Listen() (string, error) {
	ln, err := net.Listen("tcp", n.Addr)
	if err != nil {
		return "", err
	}
	n.mu.Lock()
	n.ln = ln
	n.mu.Unlock()
	go n.serve(ln)
	return ln.Addr().String(), nil
}

func (n *Node) serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		n.cMu.Lock()
		if n.closed {
			n.cMu.Unlock()
			_ = c.Close()
			return
		}
		n.conns[c] = struct{}{}
		n.cMu.Unlock()
		go n.handle(c)
	}
}

func (n *Node) handle(c net.Conn) {
	defer func() {
		n.cMu.Lock()
		delete(n.conns, c)
		n.cMu.Unlock()
		_ = c.Close()
	}()
	for {
		if n.IdleTimeout > 0 {
			_ = c.SetReadDeadline(time.Now().Add(n.IdleTimeout))
		}
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
				return // protocolo inválido: não dá para ressincronizar o stream
			}
			shard := append([]uint64(nil), wordsView(rest)...)
			n.mu.Lock()
			n.sets[name] = shard
			n.mu.Unlock()
			// sem dados na resposta (fire-and-forget), mas completa para
			// sincronia: o cliente espera 8 bytes.
			_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := writeMsg(c, make([]byte, 8)); err != nil {
				return
			}
			continue
		case opAndStored:
			a, rest, ok := readName(msg[1:])
			if !ok {
				return
			}
			b, _, ok := readName(rest)
			if !ok {
				return
			}
			n.mu.RLock()
			count = andCount(n.sets[a], n.sets[b])
			n.mu.RUnlock()
		case opAndQuery:
			name, rest, ok := readName(msg[1:])
			if !ok {
				return
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
		_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := writeMsg(c, resp[:]); err != nil {
			return
		}
	}
}

// SetCount — quantos conjuntos o nó guarda (diagnóstico).
func (n *Node) SetCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.sets)
}

// Close para de aceitar e derruba as conexões abertas (simula queda do nó e
// libera as goroutines nos testes).
func (n *Node) Close() {
	n.cMu.Lock()
	n.closed = true
	for c := range n.conns {
		_ = c.Close()
	}
	n.cMu.Unlock()

	n.mu.Lock()
	if n.ln != nil {
		_ = n.ln.Close()
		n.ln = nil
	}
	n.mu.Unlock()
}

// Coordinator fala com S nós (partição fixa por faixa de palavras).
type Coordinator struct {
	opt   Options
	peers []*PeerConn
	lo    []int
	hi    []int
}

// Dial conecta nos nós com as opções padrão (3 s dial, 10 s IO, 2 reconexões).
func Dial(addrs []string, W int) (*Coordinator, error) {
	return DialWithOptions(addrs, W, DefaultOptions())
}

// DialWithOptions — como Dial, com limites explícitos.
func DialWithOptions(addrs []string, W int, opt Options) (*Coordinator, error) {
	if len(addrs) == 0 {
		return nil, errors.New("cluster: nenhum endereço de nó")
	}
	if W < 0 {
		return nil, fmt.Errorf("cluster: W negativo (%d)", W)
	}
	opt = opt.withDefaults()
	c := &Coordinator{opt: opt}
	S := len(addrs)
	for i, a := range addrs {
		p := newPeerConn(a, opt)
		if err := p.Connect(); err != nil {
			c.Close()
			return nil, err
		}
		c.peers = append(c.peers, p)
		c.lo = append(c.lo, i*W/S)
		c.hi = append(c.hi, (i+1)*W/S)
	}
	return c, nil
}

// Peers — endereços na ordem das faixas.
func (c *Coordinator) Peers() []string {
	out := make([]string, len(c.peers))
	for i, p := range c.peers {
		out[i] = p.Addr()
	}
	return out
}

// Health — endereço → está fora? (última operação falhou).
func (c *Coordinator) Health() map[string]bool {
	out := make(map[string]bool, len(c.peers))
	for _, p := range c.peers {
		out[p.Addr()] = p.Down()
	}
	return out
}

// LoadSet envia a cada nó a fatia (do seu intervalo) do conjunto `global`.
func (c *Coordinator) LoadSet(name string, global []uint64) error {
	if err := validateName(name); err != nil {
		return err
	}
	for i, p := range c.peers {
		payload := []byte{opLoadSet}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(global[c.lo[i]:c.hi[i]])...)
		if _, err := p.roundTrip(payload); err != nil {
			return err
		}
	}
	return nil
}

func (c *Coordinator) gather(send func(i int) ([]byte, error)) (uint64, error) {
	counts := make([]uint64, len(c.peers))
	errs := make([]error, len(c.peers))
	var wg sync.WaitGroup
	for i := range c.peers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload, err := send(i)
			if err != nil {
				errs[i] = err
				return
			}
			resp, err := c.peers[i].roundTrip(payload)
			if err != nil {
				errs[i] = err
				return
			}
			if len(resp) < 8 {
				errs[i] = fmt.Errorf("cluster: resposta curta (%d bytes) de %s", len(resp), c.peers[i].Addr())
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
	if err := validateName(a); err != nil {
		return 0, err
	}
	if err := validateName(b); err != nil {
		return 0, err
	}
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
	if err := validateName(name); err != nil {
		return 0, err
	}
	if len(query) < c.hi[len(c.hi)-1] {
		return 0, fmt.Errorf("cluster: consulta com %d palavras (os nós esperam até %d)", len(query), c.hi[len(c.hi)-1])
	}
	return c.gather(func(i int) ([]byte, error) {
		payload := []byte{opAndQuery}
		payload = writeName(payload, name)
		payload = append(payload, bytesView(query[c.lo[i]:c.hi[i]])...)
		return payload, nil
	})
}

func (c *Coordinator) Close() {
	for _, p := range c.peers {
		if p != nil {
			p.Close()
		}
	}
}
