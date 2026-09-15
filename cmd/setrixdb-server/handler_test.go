// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func call(s *store, method, path, body, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	s.handle(rec, req)
	return rec
}

func TestServerFlow(t *testing.T) {
	s := newStore("", "") // só memória, sem auth

	if rec := call(s, "GET", "/health", "", ""); rec.Code != 200 {
		t.Fatalf("health: %d", rec.Code)
	}
	if rec := call(s, "PUT", "/sets/a", `{"ids":[1,2,3,4]}`, "application/json"); rec.Code != 201 {
		t.Fatalf("put a: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(s, "PUT", "/sets/b", `{"ids":[3,4,5,6]}`, "application/json"); rec.Code != 201 {
		t.Fatalf("put b: %d", rec.Code)
	}
	rec := call(s, "POST", "/intersect", `{"sets":["a","b"],"list":true}`, "application/json")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"count":2`) {
		t.Fatalf("intersect: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(s, "GET", "/sets/a/has?id=3", "", "")
	if !strings.Contains(rec.Body.String(), `"member":true`) {
		t.Fatalf("has: %s", rec.Body.String())
	}
	// texto puro (sem JSON)
	if rec := call(s, "PUT", "/sets/c", "10\n20\n30\n", "text/plain"); rec.Code != 201 || !strings.Contains(rec.Body.String(), `"count":3`) {
		t.Fatalf("put text/plain: %d %s", rec.Code, rec.Body.String())
	}
	// corpo inválido
	if rec := call(s, "POST", "/intersect", `{bad`, "application/json"); rec.Code != 400 {
		t.Fatalf("json inválido deveria dar 400, veio %d", rec.Code)
	}
	// conjunto inexistente
	if rec := call(s, "GET", "/sets/zzz", "", ""); rec.Code != 404 {
		t.Fatalf("inexistente deveria dar 404, veio %d", rec.Code)
	}
	// delete
	if rec := call(s, "DELETE", "/sets/a", "", ""); rec.Code != 200 {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestServerAuth(t *testing.T) {
	s := newStore("", "segredo")
	// sem token -> 401
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	s.handle(rec, req)
	if rec.Code != 401 {
		t.Fatalf("sem token deveria dar 401, veio %d", rec.Code)
	}
	// com token -> 200
	req = httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("Authorization", "Bearer segredo")
	rec = httptest.NewRecorder()
	s.handle(rec, req)
	if rec.Code != 200 {
		t.Fatalf("com token deveria dar 200, veio %d", rec.Code)
	}
}

func TestServerPersistenceReload(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir, "")
	call(s, "PUT", "/sets/a", `{"ids":[1,2,3]}`, "application/json")

	s2 := newStore(dir, "") // deve recarregar do disco
	rec := call(s2, "GET", "/sets/a", "", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"count":3`) {
		t.Fatalf("recarregar do disco: %d %s", rec.Code, rec.Body.String())
	}
}
