// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

//! Teste de contrato: exercita o crate como um consumidor externo (só API
//! pública), com conjuntos do tamanho de um caso real de atribuição de
//! audiência — é o cenário que justifica a existência do SetrixDB.

use setrixdb_sys::Set;

#[test]
fn audiencia_intersect_100k() {
    let universo: Vec<u64> = (0..1_000_000).collect();
    let filtro_a: Vec<u64> = universo.iter().copied().filter(|i| i % 3 == 0).collect(); // 333.333
    let filtro_b: Vec<u64> = universo.iter().copied().filter(|i| i % 5 == 0).collect(); // 200.000

    let mut a = Set::new();
    a.add_many(&filtro_a).expect("add_many");
    let mut b = Set::new();
    b.add_many(&filtro_b).expect("add_many");

    assert_eq!(a.len(), filtro_a.len());
    assert_eq!(b.len(), filtro_b.len());

    // múltiplos de 15 em [0, 1M) = 66.667 (inclui o 0: 0 é múltiplo de 15)
    assert_eq!(a.intersect_count(&b).expect("count"), 66_667);
    let ids = a.intersect_ids(&b).expect("ids");
    assert_eq!(ids.len(), 66_667);
    assert!(
        ids.windows(2).all(|w| w[0] < w[1]),
        "ids devem vir ordenados"
    );
    assert!(ids.iter().all(|i| i % 15 == 0));
}

#[test]
fn handles_nao_vazam_entre_instancias() {
    // Regressão do bug clássico de wrapper FFI: dois `Set` nunca podem
    // compartilhar o mesmo handle, senão o primeiro `Drop` invalida o segundo.
    let mut a = Set::new();
    a.add_many(&[1]).expect("add_many");
    let b = a.clone();
    drop(a);
    let mut c = Set::new();
    c.add_many(&[1]).expect("add_many");
    drop(b);
    assert_eq!(c.ids(), vec![1]);
}
