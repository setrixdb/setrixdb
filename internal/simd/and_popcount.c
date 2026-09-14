#include <immintrin.h>
#include <stdint.h>

// addb_and_popcount: popcount(a AND b) em blocos de 8 palavras uint64 (512 bits),
// usando AVX-512: vpandq + vpopcntq + redução.
long long addb_and_popcount(const uint64_t *a, const uint64_t *b, long long n) {
    __m512i acc = _mm512_setzero_si512();
    long long i = 0;
    for (; i + 8 <= n; i += 8) {
        __m512i va = _mm512_loadu_si512((const void *)(a + i));
        __m512i vb = _mm512_loadu_si512((const void *)(b + i));
        acc = _mm512_add_epi64(acc, _mm512_popcnt_epi64(_mm512_and_si512(va, vb)));
    }
    long long s = (long long)_mm512_reduce_add_epi64(acc);
    for (; i < n; i++) {
        s += __builtin_popcountll(a[i] & b[i]);
    }
    return s;
}
