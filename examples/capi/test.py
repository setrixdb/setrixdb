# Copyright (c) 2026 Thiago Silva
# SPDX-License-Identifier: Apache-2.0

"""Exemplo de uso da C ABI do SetrixDB a partir de Python (ctypes).

Antes: CGO_ENABLED=1 go build -buildmode=c-shared -o libsetrixdb.so ./cmd/setrixdb-capi
Depois: python3 test.py
"""
import ctypes

lib = ctypes.CDLL("./libsetrixdb.so")

lib.sx_version.restype = ctypes.c_char_p
lib.sx_set_new.restype = ctypes.c_longlong
lib.sx_set_add_many.argtypes = [ctypes.c_longlong, ctypes.POINTER(ctypes.c_uint64), ctypes.c_longlong]
lib.sx_set_len.argtypes = [ctypes.c_longlong]
lib.sx_set_len.restype = ctypes.c_longlong
lib.sx_set_has.argtypes = [ctypes.c_longlong, ctypes.c_uint64]
lib.sx_intersect_many.argtypes = [ctypes.c_longlong, ctypes.c_longlong]
lib.sx_intersect_many.restype = ctypes.c_longlong

a = lib.sx_set_new()
lib.sx_set_add_many(a, (ctypes.c_uint64 * 4)(1, 2, 3, 4), 4)
b = lib.sx_set_new()
lib.sx_set_add_many(b, (ctypes.c_uint64 * 4)(3, 4, 5, 6), 4)

print("version =", lib.sx_version().decode())
print("len(a) =", lib.sx_set_len(a))
print("has(3) =", lib.sx_set_has(a, 3), " has(9) =", lib.sx_set_has(a, 9))
print("|A n B| =", lib.sx_intersect_many(a, b))
