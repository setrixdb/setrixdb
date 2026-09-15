// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

/* Exemplo de uso da C ABI do SetrixDB.
 *
 * Compile a biblioteca primeiro:
 *   CGO_ENABLED=1 go build -buildmode=c-shared -o libsetrixdb.so ./cmd/setrixdb-capi
 *
 * Depois:
 *   gcc test.c -L. -lsetrixdb -o testc && LD_LIBRARY_PATH=. ./testc
 */
#include <stdio.h>
#include <stdint.h>
#include "libsetrixdb.h"

int main(void) {
    long long a = sx_set_new();
    uint64_t A[] = {1, 2, 3, 4};
    sx_set_add_many(a, A, 4);

    long long b = sx_set_new();
    uint64_t B[] = {3, 4, 5, 6};
    sx_set_add_many(b, B, 4);

    printf("version=%s  len(a)=%lld  has(3)=%d  has(9)=%d  |A n B|=%lld\n",
           sx_version(), sx_set_len(a), sx_set_has(a, 3), sx_set_has(a, 9),
           sx_intersect_many(a, b));

    uint64_t *out = NULL;
    long long n = 0;
    sx_intersect_ids(a, b, &out, &n);
    printf("ids:"); for (long long i = 0; i < n; i++) printf(" %llu", (unsigned long long)out[i]);
    printf("\n");
    if (out) sx_free(out);

    sx_set_free(a);
    sx_set_free(b);
    return 0;
}
