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
