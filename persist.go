// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package setrixdb

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
)

// fileMagic identifica o arquivo .sxset e a versão do formato.
const fileMagic = "SXSET1"

// WriteFile grava o conjunto em disco no formato `.sxset` (binário, little-endian):
//
//	magic "SXSET1" (6 bytes) · type 'S' (esparso) · n uint64 · n × uint64
//
// Persistir **conjuntos** (o índice) — e não payloads — é o que mantém o SetrixDB
// como engine/índice: a unidade salva é um conjunto de IDs.
func (s *Set) WriteFile(path string) error {
	ids := s.IDs()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	if _, err := bw.WriteString(fileMagic); err != nil {
		return err
	}
	if err := bw.WriteByte('S'); err != nil {
		return err
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(len(ids)))
	if _, err := bw.Write(buf[:]); err != nil {
		return err
	}
	for _, id := range ids {
		binary.LittleEndian.PutUint64(buf[:], id)
		if _, err := bw.Write(buf[:]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadFile carrega um conjunto gravado por WriteFile.
func ReadFile(path string) (*Set, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 15 || string(data[:6]) != fileMagic {
		return nil, fmt.Errorf("%s: não é um arquivo .sxset válido", path)
	}
	if data[6] != 'S' {
		return nil, fmt.Errorf("%s: tipo de conjunto não suportado (%q)", path, data[6])
	}
	n := binary.LittleEndian.Uint64(data[7:15])
	body := data[15:]
	if uint64(len(body)) < n*8 {
		return nil, fmt.Errorf("%s: arquivo truncado", path)
	}
	ids := make([]uint64, n)
	for i := uint64(0); i < n; i++ {
		ids[i] = binary.LittleEndian.Uint64(body[i*8 : i*8+8])
	}
	return &Set{ids: ids, sorted: true}, nil
}
