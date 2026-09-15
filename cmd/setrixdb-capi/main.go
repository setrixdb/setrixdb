// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

// Command setrixdb-capi — ABI C (FFI) do SetrixDB para embutir em C/C++/Rust/Python.
//
// Compile como biblioteca compartilhada:
//
//	CGO_ENABLED=1 go build -buildmode=c-shared -o libsetrixdb.so ./cmd/setrixdb-capi
//
// Gera também `libsetrixdb.h`. O handle é um inteiro (registro interno), o que evita
// passar ponteiros Go para C. Arrays devolvidos são alocados com malloc e liberados
// com `sx_free`.
package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/setrixdb/setrixdb"
)

var (
	mu   sync.Mutex
	reg        = map[int64]*setrixdb.Set{}
	next int64 = 1
)

//export sx_version
func sx_version() *C.char {
	return C.CString("0.1.0")
}

//export sx_set_new
func sx_set_new() C.longlong {
	mu.Lock()
	h := next
	next++
	reg[h] = setrixdb.NewSet()
	mu.Unlock()
	return C.longlong(h)
}

//export sx_set_add_many
func sx_set_add_many(h C.longlong, ids *C.uint64_t, n C.longlong) C.int {
	mu.Lock()
	set, ok := reg[int64(h)]
	mu.Unlock()
	if !ok || n <= 0 {
		return 0
	}
	sl := unsafe.Slice((*uint64)(unsafe.Pointer(ids)), int(n))
	set.Add(sl...)
	return 1
}

//export sx_set_len
func sx_set_len(h C.longlong) C.longlong {
	mu.Lock()
	set, ok := reg[int64(h)]
	mu.Unlock()
	if !ok {
		return -1
	}
	return C.longlong(set.Len())
}

//export sx_set_has
func sx_set_has(h C.longlong, id C.uint64_t) C.int {
	mu.Lock()
	set, ok := reg[int64(h)]
	mu.Unlock()
	if !ok {
		return -1
	}
	if set.Has(uint64(id)) {
		return 1
	}
	return 0
}

//export sx_intersect_many
func sx_intersect_many(a C.longlong, b C.longlong) C.longlong {
	mu.Lock()
	sa, ok1 := reg[int64(a)]
	sb, ok2 := reg[int64(b)]
	mu.Unlock()
	if !ok1 || !ok2 {
		return -1
	}
	return C.longlong(setrixdb.Intersect(sa, sb).Len())
}

// sx_intersect_ids: preenche *out com os IDs do resultado (malloc).
//
//export sx_intersect_ids
func sx_intersect_ids(a C.longlong, b C.longlong, out **C.uint64_t, outN *C.longlong) C.int {
	mu.Lock()
	sa, ok1 := reg[int64(a)]
	sb, ok2 := reg[int64(b)]
	mu.Unlock()
	if !ok1 || !ok2 {
		return 0
	}
	ids := setrixdb.Intersect(sa, sb).IDs()
	if len(ids) == 0 {
		*out = nil
		*outN = 0
		return 1
	}
	buf := C.malloc(C.size_t(len(ids)) * C.size_t(8))
	sl := unsafe.Slice((*uint64)(buf), len(ids))
	copy(sl, ids)
	*out = (*C.uint64_t)(buf)
	*outN = C.longlong(len(ids))
	return 1
}

//export sx_set_free
func sx_set_free(h C.longlong) C.int {
	mu.Lock()
	_, ok := reg[int64(h)]
	delete(reg, int64(h))
	mu.Unlock()
	if !ok {
		return 0
	}
	return 1
}

//export sx_free
func sx_free(p unsafe.Pointer) {
	C.free(p)
}

func main() {}
