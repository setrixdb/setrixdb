// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

//! `setrixdb-sys` — bindings FFI + wrapper seguro para a ABI C do SetrixDB.
//!
//! O SetrixDB é um motor de operações exatas de conjunto sobre IDs `u64`
//! (interseção em microssegundos, ao lado do banco). A ABI C expõe **handles
//! inteiros** (`i64`) em vez de ponteiros Go — o crate mantém exatamente esse
//! contrato: nada de ponteiro Go atravessando a fronteira.
//!
//! ```
//! use setrixdb_sys::Set;
//!
//! let mut a = Set::new();
//! a.add_many(&[1, 2, 3, 4]).expect("add_many");
//! let mut b = Set::new();
//! b.add_many(&[3, 4, 5]).expect("add_many");
//!
//! let i = Set::intersect_many([&a, &b]).expect("interseção");
//! assert_eq!(i.ids(), vec![3, 4]);
//! ```
//!
//! # Segurança
//!
//! Todo `unsafe` do crate está no módulo [`sys`] e nas chamadas de [`Set`]
//! (uma por método, cada uma com contrato documentado). O handle é opaco: o
//! usuário não consegue construir um [`Set`] com handle arbitrário, e o `Drop`
//! libera o registro no lado Go exatamente uma vez.
//!
//! A biblioteca compartilhada é compilada pelo `build.rs` (`go build
//! -buildmode=c-shared`) ou fornecida por `SETRIXDB_LIB`.

use std::fmt;
use std::sync::OnceLock;

/// Bindings crus da ABI C. Sem segurança nenhuma por design — use [`Set`].
pub mod sys {
    use std::os::raw::{c_char, c_int, c_longlong};

    /// Handle opaco do registro interno do lado Go.
    pub type Handle = c_longlong;

    extern "C" {
        pub fn sx_version() -> *mut c_char;
        pub fn sx_set_new() -> Handle;
        pub fn sx_set_add_many(h: Handle, ids: *const u64, n: c_longlong) -> c_int;
        pub fn sx_set_len(h: Handle) -> c_longlong;
        pub fn sx_set_has(h: Handle, id: u64) -> c_int;
        pub fn sx_intersect_many(a: Handle, b: Handle) -> c_longlong;
        pub fn sx_intersect_ids(
            a: Handle,
            b: Handle,
            out: *mut *mut u64,
            out_n: *mut c_longlong,
        ) -> c_int;
        pub fn sx_set_free(h: Handle) -> c_int;
        pub fn sx_free(p: *mut std::ffi::c_void);
    }
}

/// Erros da ponte. Handle inválido só acontece se o registro Go descartou o
/// conjunto (ex.: `Set` já liberado) — em uso normal é inalcançável.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Error {
    /// O lado Go não reconhece o handle (conjunto já liberado).
    InvalidHandle,
    /// A ABI recusou a operação (ex.: nada a adicionar).
    Abi,
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::InvalidHandle => write!(f, "handle inválido (conjunto já liberado)"),
            Error::Abi => write!(f, "a ABI C do SetrixDB recusou a operação"),
        }
    }
}

impl std::error::Error for Error {}

/// Conjunto de IDs `u64` do SetrixDB.
///
/// `Set` não é `Copy`: cada instância é um registro do lado Go e é liberada no
/// [`Drop`]. É `Clone` (cópia real dos IDs) e pode ser movido entre threads.
pub struct Set {
    handle: sys::Handle,
}

impl Set {
    /// Cria um conjunto vazio. O handle vem do registro Go e é sempre > 0.
    pub fn new() -> Self {
        // SAFETY: `sx_set_new` não tem pré-condições e devolve um handle novo
        // (o registro é protegido por mutex no lado Go).
        let handle = unsafe { sys::sx_set_new() };
        debug_assert!(handle > 0, "ABI devolveu handle inválido: {handle}");
        Set { handle }
    }

    /// Adiciona vários IDs de uma vez (uma travessia no lado Go).
    ///
    /// Lista vazia é no-op: `Ok(())` — adicionar nada não é erro de quem chama.
    pub fn add_many(&mut self, ids: &[u64]) -> Result<(), Error> {
        if ids.is_empty() {
            return Ok(());
        }
        // SAFETY: `ids` é válido para leitura por `ids.len()` elementos e o
        // Go copia os valores para o conjunto antes de retornar (nada retém o
        // ponteiro depois da chamada).
        let rc = unsafe { sys::sx_set_add_many(self.handle, ids.as_ptr(), ids.len() as i64) };
        if rc == 1 {
            Ok(())
        } else {
            Err(Set::classify(rc))
        }
    }

    /// Quantidade de IDs no conjunto.
    pub fn len(&self) -> usize {
        // SAFETY: handle pertence a esta instância viva.
        let n = unsafe { sys::sx_set_len(self.handle) };
        debug_assert!(n >= 0, "ABI devolveu len negativo: {n}");
        n.max(0) as usize
    }

    /// `true` se o conjunto está vazio.
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// `true` se o ID está no conjunto.
    pub fn contains(&self, id: u64) -> bool {
        // SAFETY: handle pertence a esta instância viva.
        unsafe { sys::sx_set_has(self.handle, id) == 1 }
    }

    /// Interseção com outro conjunto, devolvendo apenas a **cardinalidade**
    /// (o caminho mais barato: nada é alocado do lado Rust).
    pub fn intersect_count(&self, other: &Set) -> Result<usize, Error> {
        // SAFETY: os dois handles pertencem a instâncias vivas.
        let n = unsafe { sys::sx_intersect_many(self.handle, other.handle) };
        if n < 0 {
            Err(Error::InvalidHandle)
        } else {
            Ok(n as usize)
        }
    }

    /// Interseção com outro conjunto, devolvendo os IDs (ordenados).
    ///
    /// O buffer vem de `malloc` no lado C; ele é copiado para um `Vec` e
    /// liberado com `sx_free` **antes** de retornar — o chamador nunca vê
    /// memória da ABI.
    pub fn intersect_ids(&self, other: &Set) -> Result<Vec<u64>, Error> {
        let mut out: *mut u64 = std::ptr::null_mut();
        let mut n: i64 = 0;
        // SAFETY: handles vivos; `out`/`n` são ponteiros para locais válidos e a
        // ABI só escreve neles (o buffer devolvido é nosso para liberar).
        let rc = unsafe { sys::sx_intersect_ids(self.handle, other.handle, &mut out, &mut n) };
        if rc != 1 {
            return Err(Error::InvalidHandle);
        }
        if out.is_null() || n <= 0 {
            return Ok(Vec::new());
        }
        // SAFETY: a ABI garante `n` elementos `u64` em `out`.
        let ids = unsafe { std::slice::from_raw_parts(out, n as usize) }.to_vec();
        // SAFETY: buffer alocado por `malloc` na ABI; `sx_free` é o par dela.
        unsafe { sys::sx_free(out as *mut std::ffi::c_void) };
        Ok(ids)
    }

    /// Interseção de N conjuntos (`N >= 1`), devolvendo um conjunto novo.
    ///
    /// A ABI C é binária, então a interseção múltipla é feita em cadeia: o
    /// resultado parcial vira conjunto e entra na próxima etapa. Lista vazia
    /// devolve conjunto vazio (identidade da interseção); um único conjunto é
    /// clonado.
    pub fn intersect_many<'a, I>(sets: I) -> Result<Set, Error>
    where
        I: IntoIterator<Item = &'a Set>,
    {
        let mut it = sets.into_iter();
        let first = match it.next() {
            Some(s) => s,
            None => return Ok(Set::new()),
        };
        let mut acc = first.clone_set()?;
        for s in it {
            let ids = acc.intersect_ids(s)?;
            if ids.is_empty() {
                return Ok(Set::new());
            }
            let mut next = Set::new();
            next.add_many(&ids)?;
            acc = next; // o Drop do `acc` anterior libera o handle intermediário
        }
        Ok(acc)
    }

    /// Todos os IDs do conjunto, ordenados.
    ///
    /// A ABI não expõe "listar", só interseção — então `A ∩ A` é o caminho
    /// (mesma cardinalidade, custo de uma interseção, sem API nova do lado Go).
    pub fn ids(&self) -> Vec<u64> {
        self.intersect_ids(self).unwrap_or_default()
    }

    /// Cópia real do conjunto (novo handle, mesmos IDs).
    pub fn clone_set(&self) -> Result<Set, Error> {
        let ids = self.intersect_ids(self)?;
        let mut s = Set::new();
        s.add_many(&ids)?;
        Ok(s)
    }

    /// Traduz o `int` da ABI: `1` é sucesso; `0`/`-1` significam handle
    /// ausente no registro Go (a ABI devolve 0 também para "nada a fazer", que
    /// já é filtrado antes da chamada).
    fn classify(rc: i32) -> Error {
        if rc <= 0 {
            Error::InvalidHandle
        } else {
            Error::Abi
        }
    }
}

impl Default for Set {
    fn default() -> Self {
        Set::new()
    }
}

impl Clone for Set {
    /// Cópia real: novo handle no lado Go + `add_many`. Nunca compartilha o
    /// registro (compartilhar handle faria um `Drop` liberar o conjunto do
    /// outro — o bug clássico de wrapper FFI).
    fn clone(&self) -> Self {
        self.clone_set().expect("clone de conjunto válido")
    }
}

impl fmt::Debug for Set {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let ids = self.ids();
        // Conjunto grande não pode imprimir 10 milhões de IDs num `{:?}`.
        if ids.len() > 16 {
            write!(
                f,
                "Set {{ len: {}, ids: [{}, ...] }}",
                ids.len(),
                ids[..4]
                    .iter()
                    .map(|i| i.to_string())
                    .collect::<Vec<_>>()
                    .join(", ")
            )
        } else {
            write!(f, "Set {{ len: {}, ids: {:?} }}", ids.len(), ids)
        }
    }
}

impl Drop for Set {
    fn drop(&mut self) {
        if self.handle > 0 {
            // SAFETY: o handle é nosso (criado em `new`/`clone`) e nunca é
            // liberado duas vezes: `Drop` roda uma vez por instância.
            unsafe { sys::sx_set_free(self.handle) };
            self.handle = -1; // impede uso duplo caso algum dia vire `ManuallyDrop`
        }
    }
}

// SAFETY: o handle é um índice num registro protegido por mutex no lado Go —
// não carrega ponteiro para memória thread-local nem para o heap do Rust.
// Mover/emprestar `Set` entre threads é seguro; operações concorrentes no MESMO
// handle são serializadas pelo mutex da ABI.
unsafe impl Send for Set {}
unsafe impl Sync for Set {}

/// Versão da ABI C (`sx_version`), copiada e liberada — chamada uma vez.
pub fn version() -> &'static str {
    static VERSION: OnceLock<String> = OnceLock::new();
    VERSION.get_or_init(|| {
        // SAFETY: `sx_version` devolve CString alocada com malloc (par = sx_free).
        unsafe {
            let p = sys::sx_version();
            if p.is_null() {
                return "desconhecida".to_string();
            }
            let s = std::ffi::CStr::from_ptr(p).to_string_lossy().into_owned();
            sys::sx_free(p as *mut std::ffi::c_void);
            s
        }
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn new_set_is_empty() {
        let s = Set::new();
        assert_eq!(s.len(), 0);
        assert!(s.is_empty());
        assert!(!s.contains(1));
    }

    #[test]
    fn add_many_and_contains() {
        let mut s = Set::new();
        s.add_many(&[10, 20, 30]).expect("add_many");
        assert_eq!(s.len(), 3);
        assert!(s.contains(20));
        assert!(!s.contains(21));
        // idempotente: ID repetido não infla o conjunto
        s.add_many(&[20, 20, 40]).expect("add_many repetido");
        assert_eq!(s.len(), 4);
    }

    #[test]
    fn empty_add_many_is_noop() {
        let mut s = Set::new();
        assert!(s.add_many(&[]).is_ok());
        assert_eq!(s.len(), 0);
    }

    #[test]
    fn intersect_counts_and_ids() {
        let mut a = Set::new();
        a.add_many(&[1, 2, 3, 4, 5]).expect("add_many");
        let mut b = Set::new();
        b.add_many(&[4, 5, 6, 7]).expect("add_many");

        assert_eq!(a.intersect_count(&b).expect("count"), 2);
        assert_eq!(a.intersect_ids(&b).expect("ids"), vec![4, 5]);
        assert_eq!(a.ids(), vec![1, 2, 3, 4, 5]);
        // interseção não muta as entradas
        assert_eq!(a.len(), 5);
        assert_eq!(b.len(), 4);
    }

    #[test]
    fn intersect_disjoint_is_empty() {
        let mut a = Set::new();
        a.add_many(&[1, 2]).expect("add_many");
        let mut b = Set::new();
        b.add_many(&[9]).expect("add_many");
        assert!(a.intersect_ids(&b).expect("ids").is_empty());
        assert_eq!(a.intersect_count(&b).expect("count"), 0);
    }

    #[test]
    fn intersect_many_of_three() {
        let mut a = Set::new();
        a.add_many(&[1, 2, 3, 4]).expect("add_many");
        let mut b = Set::new();
        b.add_many(&[2, 3, 4, 5]).expect("add_many");
        let mut c = Set::new();
        c.add_many(&[3, 4, 6]).expect("add_many");

        let r = Set::intersect_many([&a, &b, &c]).expect("intersect_many");
        assert_eq!(r.ids(), vec![3, 4]);
    }

    #[test]
    fn intersect_many_edge_cases() {
        let _vazio = Set::new();
        let none: [&Set; 0] = [];
        assert!(Set::intersect_many(none).expect("vazio").is_empty());

        let mut a = Set::new();
        a.add_many(&[7, 8]).expect("add_many");
        let one = Set::intersect_many([&a]).expect("um conjunto");
        assert_eq!(one.ids(), vec![7, 8]);
    }

    #[test]
    fn clone_is_deep_and_drop_is_safe() {
        let mut a = Set::new();
        a.add_many(&[1, 2, 3]).expect("add_many");
        let b = a.clone();
        drop(a); // libera o handle original
                 // o clone continua válido: handles diferentes
        assert_eq!(b.ids(), vec![1, 2, 3]);
    }

    #[test]
    fn version_is_reported() {
        assert!(!version().is_empty());
        // cacheado: segunda chamada devolve o mesmo &'static str
        assert_eq!(version(), version());
    }

    #[test]
    fn debug_truncates_large_set() {
        let mut a = Set::new();
        let ids: Vec<u64> = (0..100).collect();
        a.add_many(&ids).expect("add_many");
        let d = format!("{a:?}");
        assert!(d.contains("len: 100"), "{d}");
        assert!(d.contains("..."), "{d}");
    }
}
