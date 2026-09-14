package addb

import (
	"hash/fnv"
	"sort"
)

// Ring é um Consistent Hash Ring: roteia IDs uint64 para nós do cluster sem
// re-hash total quando nós entram ou saem. Base da topologia distribuída do ADDB.
type Ring struct {
	// replicas é o número de pontos virtuais por nó (melhora o balanceamento).
	replicas int
	// points mapeia posição no anel -> nome do nó.
	points map[uint64]string
	// nodes guarda os nós ativos.
	nodes map[string]struct{}
	// sorted é o cache ordenado das posições (lazy).
	sorted []uint64
}

// NewRing cria um anel com o número de réplicas virtuais indicado.
// replicas <= 0 usa o padrão 100.
func NewRing(replicas int) *Ring {
	if replicas <= 0 {
		replicas = 100
	}
	return &Ring{
		replicas: replicas,
		points:   make(map[uint64]string),
		nodes:    make(map[string]struct{}),
	}
}

// hashKey mapeia um ID (ou chave de nó) para uma posição no anel.
// Usa FNV-1a por ser determinístico e barato; pode ser trocado por xxhash.
func hashKey(id uint64) uint64 {
	h := fnv.New64a()
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(id >> (8 * i))
	}
	_, _ = h.Write(b[:])
	return h.Sum64()
}

// AddNode insere um nó e seus pontos virtuais no anel.
func (r *Ring) AddNode(node string) {
	if _, exists := r.nodes[node]; exists {
		return
	}
	r.nodes[node] = struct{}{}
	for i := 0; i < r.replicas; i++ {
		r.points[hashKeyNode(node, i)] = node
	}
	r.sorted = nil
}

// RemoveNode remove um nó e seus pontos virtuais do anel.
func (r *Ring) RemoveNode(node string) {
	if _, exists := r.nodes[node]; !exists {
		return
	}
	delete(r.nodes, node)
	for i := 0; i < r.replicas; i++ {
		delete(r.points, hashKeyNode(node, i))
	}
	r.sorted = nil
}

// Route devolve o nó responsável por um ID. ok=false se o anel estiver vazio.
func (r *Ring) Route(id uint64) (node string, ok bool) {
	if len(r.points) == 0 {
		return "", false
	}
	r.ensureSorted()

	pos := hashKey(id)
	idx := sort.Search(len(r.sorted), func(i int) bool {
		return r.sorted[i] >= pos
	})
	if idx == len(r.sorted) {
		idx = 0 // wrap-around do anel
	}
	return r.points[r.sorted[idx]], true
}

// Nodes devolve a lista de nós ativos (ordem alfabética).
func (r *Ring) Nodes() []string {
	out := make([]string, 0, len(r.nodes))
	for n := range r.nodes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (r *Ring) ensureSorted() {
	if r.sorted != nil {
		return
	}
	r.sorted = make([]uint64, 0, len(r.points))
	for p := range r.points {
		r.sorted = append(r.sorted, p)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return r.sorted[i] < r.sorted[j] })
}

// hashKeyNode gera a posição do i-ésimo ponto virtual de um nó.
func hashKeyNode(node string, i int) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(node))
	_, _ = h.Write([]byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)})
	return h.Sum64()
}
