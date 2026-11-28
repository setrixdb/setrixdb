// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0
//
// build.rs — compila a ABI C do SetrixDB (`cmd/setrixdb-capi`) como
// `libsetrixdb.so` e liga o crate nela.
//
// Duas formas de obter a biblioteca, nesta ordem:
//   1. SETRIXDB_LIB=/caminho/libsetrixdb.so  (biblioteca já compilada — CI sem Go)
//   2. `go build -buildmode=c-shared` no repo (padrão; exige Go 1.21+ com cgo)
//
// Falhar com mensagem explícita é parte do contrato: build que "passa" sem a
// biblioteca produz um binário que estoura em runtime no primeiro `Set::new`.

use std::env;
use std::path::{Path, PathBuf};
use std::process::Command;

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR"));
    let repo = manifest
        .parent()
        .expect("rust/ deve ficar na raiz do repo setrixdb")
        .to_path_buf();
    let out = PathBuf::from(env::var("OUT_DIR").expect("OUT_DIR"));

    rerun_if_changed(&repo);

    let lib = match env::var("SETRIXDB_LIB") {
        Ok(p) if !p.trim().is_empty() => {
            let p = PathBuf::from(p);
            assert!(
                p.is_file(),
                "SETRIXDB_LIB aponta para algo que não é arquivo: {}",
                p.display()
            );
            println!("cargo:rerun-if-env-changed=SETRIXDB_LIB");
            p
        }
        _ => build_with_go(&repo, &out),
    };

    let dir = lib.parent().expect("lib sem diretório");
    println!("cargo:rustc-link-search=native={}", dir.display());
    println!("cargo:rustc-link-lib=dylib=setrixdb");
    // rpath: sem isto `cargo test` acha a lib no build e o binário não acha em
    // runtime (o clássico "funciona no CI, quebra no deploy").
    println!("cargo:rustc-link-arg=-Wl,-rpath,{}", dir.display());
}

/// Recompila quando o lado Go mudar (ABI é contrato dos dois lados).
fn rerun_if_changed(repo: &Path) {
    println!(
        "cargo:rerun-if-changed={}",
        repo.join("cmd/setrixdb-capi").display()
    );
    println!("cargo:rerun-if-changed={}", repo.join("go.mod").display());
    if let Ok(entries) = std::fs::read_dir(repo) {
        for e in entries.flatten() {
            let p = e.path();
            if p.extension().map(|x| x == "go").unwrap_or(false) {
                println!("cargo:rerun-if-changed={}", p.display());
            }
        }
    }
    println!("cargo:rerun-if-changed=build.rs");
}

fn build_with_go(repo: &Path, out: &Path) -> PathBuf {
    let go = env::var("GO").unwrap_or_else(|_| "go".to_string());
    let so = out.join("libsetrixdb.so");

    let output = Command::new(&go)
        .args([
            "build",
            "-buildmode=c-shared",
            "-o",
            so.to_str().expect("OUT_DIR não-UTF8"),
            "./cmd/setrixdb-capi",
        ])
        .current_dir(repo)
        .env("CGO_ENABLED", "1")
        .output();

    match output {
        Ok(o) if o.status.success() => so,
        Ok(o) => panic!(
            "falha ao compilar a ABI C do SetrixDB com `{go} build -buildmode=c-shared`:\n{}\n{}\n\
             Dica: Go 1.21+ com cgo (gcc) — ou defina SETRIXDB_LIB apontando para um libsetrixdb.so pronto.",
            String::from_utf8_lossy(&o.stdout).trim(),
            String::from_utf8_lossy(&o.stderr).trim()
        ),
        Err(e) => panic!(
            "não consegui executar `{go}` ({e}). Instale Go 1.21+ (com cgo) ou defina \
             SETRIXDB_LIB=/caminho/libsetrixdb.so para usar uma biblioteca já compilada."
        ),
    }
}
