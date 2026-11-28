# setrixdb-sys — bindings Rust

Wrapper seguro sobre a ABI C do SetrixDB (`libsetrixdb.so`). A ABI usa
**handles inteiros**, então nenhum ponteiro Go atravessa a fronteira.

```rust
use setrixdb_sys::Set;

let mut a = Set::new();
a.add_many(&[1, 2, 3, 4]);
let mut b = Set::new();
b.add_many(&[3, 4, 5]);

assert_eq!(Set::intersect_many([&a, &b]).unwrap().ids(), vec![3, 4]);
```

## API

| item | contrato |
|---|---|
| `Set::new()` | conjunto vazio (handle novo no registro Go) |
| `add_many(&[u64])` | insere em lote; lista vazia é no-op |
| `len()` / `is_empty()` / `contains(u64)` | leitura |
| `intersect_count(&Set)` | cardinalidade (sem alocar no Rust) |
| `intersect_ids(&Set)` | IDs da interseção, ordenados (buffer `malloc` copiado e liberado) |
| `Set::intersect_many([&Set; N])` | interseção de N conjuntos, em cadeia |
| `ids()` | todos os IDs, ordenados |
| `clone()` / `clone_set()` | cópia real (handle novo — nunca compartilha registro) |
| `Drop` | `sx_set_free` uma vez por instância |
| `version()` | versão da ABI (`sx_version`), cacheada |

`Set` é `Send + Sync`: o handle é índice de um registro protegido por mutex no
lado Go.

## Build

O `build.rs` compila a ABI C na hora:

```sh
cargo test                       # exige Go 1.21+ com cgo (gcc)
```

Sem Go no ambiente (CI, cross-build), aponte para uma biblioteca já compilada:

```sh
CGO_ENABLED=1 go build -buildmode=c-shared -o libsetrixdb.so ./cmd/setrixdb-capi
SETRIXDB_LIB=$PWD/libsetrixdb.so cargo test
```

O `build.rs` emite `rerun-if-changed` para `cmd/setrixdb-capi/`, `go.mod` e os
`.go` da raiz: mudou a ABI, recompila.

## Testes

- unidade (`src/lib.rs`): ciclo de vida, handles, interseção, `Drop`, `Clone`;
- integração (`tests/ffi_contract.rs`): 1M de IDs, 66.666 na interseção,
  ordenação, e regressão de handle vazando entre instâncias.
