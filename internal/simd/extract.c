// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

#include <immintrin.h>
#include <stdint.h>

// addb_extract_set: extrai, em ordem crescente, as posições (índices absolutos) dos
// bits setados do bitset `words`.
//
// PORTÁVEL: o arquivo compila SEM exigir AVX-512; a escolha entre a versão AVX-512
// (que pula palavras zeradas rápido) e o fallback escalar é feita EM RUNTIME via
// __builtin_cpu_supports. Sem isso, uma CPU sem AVX-512 sofreria SIGILL.

static size_t extract_scalar(const uint64_t *words, size_t nwords, uint64_t *out) {
    size_t cnt = 0;
    for (size_t i = 0; i < nwords; i++) {
        uint64_t w = words[i];
        while (w) {
            int p = __builtin_ctzll(w);
            out[cnt++] = (uint64_t)i * 64 + (uint64_t)p;
            w &= w - 1;
        }
    }
    return cnt;
}

__attribute__((target("avx512f")))
static size_t extract_avx512(const uint64_t *words, size_t nwords, uint64_t *out) {
    size_t cnt = 0;
    size_t i = 0;
    for (; i + 8 <= nwords; i += 8) {
        __m512i v = _mm512_loadu_si512((const void *)(words + i));
        __mmask8 m = _mm512_cmpneq_epu64_mask(v, _mm512_setzero_si512());
        while (m) {
            int b = __builtin_ctz((unsigned)m);
            uint64_t w = words[i + b];
            while (w) {
                int p = __builtin_ctzll(w);
                out[cnt++] = (uint64_t)(i + b) * 64 + (uint64_t)p;
                w &= w - 1;
            }
            m &= m - 1;
        }
    }
    for (; i < nwords; i++) {
        uint64_t w = words[i];
        while (w) {
            int p = __builtin_ctzll(w);
            out[cnt++] = (uint64_t)i * 64 + (uint64_t)p;
            w &= w - 1;
        }
    }
    return cnt;
}

// Dispatcher (híbrido): escolhe a melhor versão UMA vez e reusa.
size_t addb_extract_set(const uint64_t *words, size_t nwords, uint64_t *out) {
    static int tier = -1;
    if (tier < 0) {
        __builtin_cpu_init();
        tier = __builtin_cpu_supports("avx512f") ? 1 : 0;
    }
    return tier == 1 ? extract_avx512(words, nwords, out) : extract_scalar(words, nwords, out);
}
