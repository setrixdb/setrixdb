// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

#include <immintrin.h>
#include <stdint.h>

// Kernel AVX-512 (vpandq + vpopcntq). Compilado com target attribute próprio,
// para o restante do arquivo continuar PORTÁVEL (sem exigir AVX-512).
__attribute__((target("avx512f,avx512vpopcntdq")))
static long long and_popcount_avx512(const uint64_t *a, const uint64_t *b, long long n) {
    __m512i acc = _mm512_setzero_si512();
    long long i = 0;
    for (; i + 8 <= n; i += 8) {
        __m512i va = _mm512_loadu_si512((const void *)(a + i));
        __m512i vb = _mm512_loadu_si512((const void *)(b + i));
        acc = _mm512_add_epi64(acc, _mm512_popcnt_epi64(_mm512_and_si512(va, vb)));
    }
    long long s = (long long)_mm512_reduce_add_epi64(acc);
    for (; i < n; i++) s += __builtin_popcountll(a[i] & b[i]);
    return s;
}

// Fallback escalar (qualquer CPU).
static long long and_popcount_scalar(const uint64_t *a, const uint64_t *b, long long n) {
    long long s = 0;
    for (long long i = 0; i < n; i++) s += __builtin_popcountll(a[i] & b[i]);
    return s;
}

// Tier de runtime: 1 = AVX-512 disponível, 0 = escalar.
int addb_and_popcount_tier(void) {
    __builtin_cpu_init();
    return (__builtin_cpu_supports("avx512f") && __builtin_cpu_supports("avx512vpopcntdq")) ? 1 : 0;
}

// Dispatcher (hybrid): escolhe a melhor versão UMA vez e reusa.
long long addb_and_popcount(const uint64_t *a, const uint64_t *b, long long n) {
    static int tier = -1;
    if (tier < 0) tier = addb_and_popcount_tier();
    return tier == 1 ? and_popcount_avx512(a, b, n) : and_popcount_scalar(a, b, n);
}

// ─────────────────────────────────────────────────────────────────────────────
// AND vetorizado (dst = a & b). Usado pelo caminho denso do produto
// (addb.Bitset.Intersect e a seleção denso/esparso de setrixdb.Intersect).
// ─────────────────────────────────────────────────────────────────────────────

__attribute__((target("avx512f")))
static void and_avx512(const uint64_t *a, const uint64_t *b, uint64_t *dst, long long n) {
    long long i = 0;
    for (; i + 8 <= n; i += 8) {
        __m512i va = _mm512_loadu_si512((const void *)(a + i));
        __m512i vb = _mm512_loadu_si512((const void *)(b + i));
        _mm512_storeu_si512((void *)(dst + i), _mm512_and_si512(va, vb));
    }
    for (; i < n; i++) dst[i] = a[i] & b[i];
}

static void and_scalar(const uint64_t *a, const uint64_t *b, uint64_t *dst, long long n) {
    for (long long i = 0; i < n; i++) dst[i] = a[i] & b[i];
}

// Dispatcher do AND: b pode ser menor que a (o resto de dst fica com a).
void addb_and(const uint64_t *a, const uint64_t *b, uint64_t *dst, long long n) {
    static int tier = -1;
    if (tier < 0) tier = addb_and_popcount_tier();
    if (tier == 1) and_avx512(a, b, dst, n);
    else and_scalar(a, b, dst, n);
}

// ─────────────────────────────────────────────────────────────────────────────
// Popcount de um bitset. Serve para dimensionar a saída do extract com
// exatidão, em vez de alocar 64 posições por palavra (desperdício que tornava o
// caminho denso mais lento que o merge — medido na v1.1).
// ─────────────────────────────────────────────────────────────────────────────

__attribute__((target("avx512f,avx512vpopcntdq")))
static long long popcount_avx512(const uint64_t *a, long long n) {
    __m512i acc = _mm512_setzero_si512();
    long long i = 0;
    for (; i + 8 <= n; i += 8) {
        __m512i va = _mm512_loadu_si512((const void *)(a + i));
        acc = _mm512_add_epi64(acc, _mm512_popcnt_epi64(va));
    }
    long long s = (long long)_mm512_reduce_add_epi64(acc);
    for (; i < n; i++) s += __builtin_popcountll(a[i]);
    return s;
}

static long long popcount_scalar(const uint64_t *a, long long n) {
    long long s = 0;
    for (long long i = 0; i < n; i++) s += __builtin_popcountll(a[i]);
    return s;
}

long long addb_popcount(const uint64_t *a, long long n) {
    static int tier = -1;
    if (tier < 0) tier = addb_and_popcount_tier();
    return tier == 1 ? popcount_avx512(a, n) : popcount_scalar(a, n);
}
