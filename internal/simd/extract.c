#include <immintrin.h>
#include <stdint.h>

// addb_extract_set: extrai, em ordem crescente, as posições (índices absolutos)
// dos bits setados do bitset `words`. Usa AVX-512 para pular rápido as palavras
// zeradas; dentro de cada palavra usa trailing-zeros (ctz) + limpeza do bit.
size_t addb_extract_set(const uint64_t *words, size_t nwords, uint64_t *out) {
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
