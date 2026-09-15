// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

// Command setrixdb-server — API remota (HTTP/JSON) do SetrixDB.
//
// Sobe um servidor que mantém conjuntos nomeados em memória e expõe as operações
// do motor pela rede:
//
//	GET    /health                     -> status
//	GET    /sets                       -> lista os conjuntos
//	PUT    /sets/{name}                -> carrega/cria um conjunto   {"ids":[1,2,3]}
//	GET    /sets/{name}                -> info (count, memória)
//	DELETE /sets/{name}                -> remove
//	GET    /sets/{name}/has?id=42      -> pertencimento
//	POST   /intersect                  -> {"sets":["a","b"],"list":false} -> count
//	POST   /union                      -> {"sets":["a","b"]}             -> count
//
// Uso:
//
//	go run ./cmd/setrixdb-server -addr :8080
//	curl -X PUT localhost:8080/sets/a -d '{"ids":[1,2,3]}'
//	curl -X PUT localhost:8080/sets/b -d '{"ids":[3,4,5]}'
//	curl -X POST localhost:8080/intersect -d '{"sets":["a","b"]}'
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/setrixdb/setrixdb"
)

const version = "0.1.0"

type store struct {
	mu      sync.RWMutex
	sets    map[string]*setrixdb.Set
	dataDir string // "" = somente memória
}

// newStore cria o armazenamento e, se houver diretório, carrega os conjuntos já salvos.
func newStore(dataDir string) *store {
	s := &store{sets: map[string]*setrixdb.Set{}, dataDir: dataDir}
	s.loadFromDisk()
	return s
}

// loadFromDisk carrega os `.sxset` existentes (persistência de CONJUNTOS, não de payload).
func (s *store) loadFromDisk() {
	if s.dataDir == "" {
		return
	}
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		log.Printf("sem conjuntos em %s (%v)", s.dataDir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sxset") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".sxset")
		set, err := setrixdb.ReadFile(filepath.Join(s.dataDir, e.Name()))
		if err != nil {
			log.Printf("falha ao carregar %s: %v", e.Name(), err)
			continue
		}
		s.sets[name] = set
	}
	log.Printf("carregados %d conjuntos de %s", len(s.sets), s.dataDir)
}

// persist grava o conjunto em disco (no-op se a persistência estiver desligada).
func (s *store) persist(name string, set *setrixdb.Set) {
	if s.dataDir == "" {
		return
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		log.Printf("erro ao criar dir de dados: %v", err)
		return
	}
	if err := set.WriteFile(filepath.Join(s.dataDir, name+".sxset")); err != nil {
		log.Printf("erro ao persistir %s: %v", name, err)
	}
}

// forget remove o arquivo persistido do conjunto.
func (s *store) forget(name string) {
	if s.dataDir == "" {
		return
	}
	os.Remove(filepath.Join(s.dataDir, name+".sxset"))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *store) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		writeJSON(w, 200, map[string]string{"status": "ok", "version": version, "engine": "setrixdb"})

	case r.Method == http.MethodGet && r.URL.Path == "/sets":
		s.mu.RLock()
		names := make([]string, 0, len(s.sets))
		for n := range s.sets {
			names = append(names, n)
		}
		s.mu.RUnlock()
		writeJSON(w, 200, map[string]any{"sets": names, "count": len(names)})

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/sets/"):
		name := strings.TrimPrefix(r.URL.Path, "/sets/")
		if name == "" {
			errJSON(w, 400, "nome do conjunto obrigatório")
			return
		}
		ids, err := parseIDs(r)
		if err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		set := setrixdb.NewSet(ids...)
		s.mu.Lock()
		s.sets[name] = set
		s.mu.Unlock()
		s.persist(name, set)
		writeJSON(w, 201, map[string]any{"name": name, "count": set.Len(), "persisted": s.dataDir != ""})

	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/has"):
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/sets/"), "/has")
		s.mu.RLock()
		set, ok := s.sets[name]
		s.mu.RUnlock()
		if !ok {
			errJSON(w, 404, "conjunto não encontrado: "+name)
			return
		}
		q, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			errJSON(w, 400, "parâmetro id inválido")
			return
		}
		writeJSON(w, 200, map[string]any{"id": q, "member": set.Has(q), "set": name})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sets/"):
		name := strings.TrimPrefix(r.URL.Path, "/sets/")
		s.mu.RLock()
		set, ok := s.sets[name]
		s.mu.RUnlock()
		if !ok {
			errJSON(w, 404, "conjunto não encontrado: "+name)
			return
		}
		writeJSON(w, 200, map[string]any{"name": name, "count": set.Len()})

	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sets/"):
		name := strings.TrimPrefix(r.URL.Path, "/sets/")
		s.mu.Lock()
		_, ok := s.sets[name]
		delete(s.sets, name)
		s.mu.Unlock()
		if !ok {
			errJSON(w, 404, "conjunto não encontrado: "+name)
			return
		}
		s.forget(name)
		writeJSON(w, 200, map[string]any{"deleted": name})

	case r.Method == http.MethodPost && (r.URL.Path == "/intersect" || r.URL.Path == "/union"):
		var req struct {
			Sets []string `json:"sets"`
			List bool     `json:"list"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errJSON(w, 400, "corpo JSON inválido: "+err.Error())
			return
		}
		if len(req.Sets) < 2 {
			errJSON(w, 400, "informe ao menos 2 conjuntos")
			return
		}
		s.mu.RLock()
		parts := make([]*setrixdb.Set, 0, len(req.Sets))
		for _, n := range req.Sets {
			set, ok := s.sets[n]
			if !ok {
				s.mu.RUnlock()
				errJSON(w, 404, "conjunto não encontrado: "+n)
				return
			}
			parts = append(parts, set)
		}
		s.mu.RUnlock()

		var res *setrixdb.Set
		if r.URL.Path == "/intersect" {
			res = setrixdb.Intersect(parts...)
		} else {
			res = setrixdb.Union(parts...)
		}
		resp := map[string]any{"count": res.Len(), "sets": req.Sets}
		if req.List {
			resp["ids"] = res.IDs()
		}
		writeJSON(w, 200, resp)

	default:
		errJSON(w, 404, "rota não encontrada: "+r.Method+" "+r.URL.Path)
	}
}

func parseIDs(r *http.Request) ([]uint64, error) {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var body struct {
			IDs []uint64 `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("JSON inválido: %w", err)
		}
		return body.IDs, nil
	}
	// text/plain: um ID por linha, ou separado por vírgula/espaço
	var ids []uint64
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 64*1024)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	for _, tok := range strings.FieldsFunc(string(buf), func(c rune) bool {
		return c == ',' || c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == ';'
	}) {
		if v, e := strconv.ParseUint(tok, 10, 64); e == nil {
			ids = append(ids, v)
		}
	}
	return ids, nil
}

func main() {
	addr := flag.String("addr", ":8080", "endereço de escuta")
	data := flag.String("data", "", "diretório para persistir conjuntos (.sxset); vazio = só memória")
	flag.Parse()

	s := newStore(*data)
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)

	log.Printf("SetrixDB API v%s escutando em %s", version, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
