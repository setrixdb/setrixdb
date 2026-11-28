// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

//! Exemplo: interseção de três segmentos de audiência.
//!
//!     cargo run --example audiencia

use setrixdb_sys::Set;

fn main() {
    println!("SetrixDB ABI {}", setrixdb_sys::version());

    let mut ativos_30d = Set::new();
    ativos_30d
        .add_many(&(0..1_000_000).collect::<Vec<_>>())
        .expect("add_many");

    let mut compradores = Set::new();
    compradores
        .add_many(&(0..1_000_000).filter(|i| i % 100 == 0).collect::<Vec<_>>())
        .expect("add_many");

    let mut sp = Set::new();
    sp.add_many(&(0..1_000_000).filter(|i| i % 7 == 0).collect::<Vec<_>>())
        .expect("add_many");

    let alvo = Set::intersect_many([&ativos_30d, &compradores, &sp]).expect("interseção");
    println!(
        "ativos={} compradores={} sp={} -> alvo={}",
        ativos_30d.len(),
        compradores.len(),
        sp.len(),
        alvo.len()
    );
    println!(
        "primeiros 5 do alvo: {:?}",
        &alvo.ids()[..5.min(alvo.len())]
    );
}
