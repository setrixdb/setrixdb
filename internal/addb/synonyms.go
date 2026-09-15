// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

// SynonymEntry descreve um termo e seus sinônimos, já resolvidos em IDs uint64.
type SynonymEntry struct {
	Term     uint64
	Synonyms []uint64
}

// FlatSynonymStorage armazena a base de sinônimos em **vetores contínuos de RAM**,
// sem usar maps nativos — apenas flat arrays + índices de offset:
//
//	SynonymIDs: [ s0 s1 s2 | s3 s4 | ... ]   (todos os sinônimos concatenados)
//	Offsets:    [ 0        3      ...   ]    (início da fatia de cada termo)
//	Lengths:    [ 3        2      ...   ]    (quantos sinônimos cada termo tem)
//
// Os três arrays são paralelos: a fatia de sinônimos do termo k é
// SynonymIDs[Offsets[k] : Offsets[k]+Lengths[k]].
type FlatSynonymStorage struct {
	SynonymIDs []uint64 // array contínuo contendo todos os IDs de sinônimos
	Offsets    []uint32 // posição de início no array de sinônimos
	Lengths    []uint32 // quantidade de sinônimos por termo
	Terms      []uint64 // termo dono de cada fatia (paralelo a Offsets/Lengths)
}

// NewSynonymStorage constrói o armazenamento flat a partir das entradas.
func NewSynonymStorage(entries []SynonymEntry) *FlatSynonymStorage {
	s := &FlatSynonymStorage{
		SynonymIDs: make([]uint64, 0),
		Offsets:    make([]uint32, 0, len(entries)),
		Lengths:    make([]uint32, 0, len(entries)),
		Terms:      make([]uint64, 0, len(entries)),
	}
	for _, e := range entries {
		s.Offsets = append(s.Offsets, uint32(len(s.SynonymIDs)))
		s.Lengths = append(s.Lengths, uint32(len(e.Synonyms)))
		s.Terms = append(s.Terms, e.Term)
		s.SynonymIDs = append(s.SynonymIDs, e.Synonyms...)
	}
	return s
}

// Synonyms devolve a fatia de sinônimos de um termo (nil se não houver).
// Busca linear sobre Terms — em produção, ordene Terms e use busca binária.
func (s *FlatSynonymStorage) Synonyms(term uint64) []uint64 {
	if s == nil {
		return nil
	}
	for k, t := range s.Terms {
		if t != term {
			continue
		}
		lo := s.Offsets[k]
		hi := lo + s.Lengths[k]
		return s.SynonymIDs[lo:hi]
	}
	return nil
}

// Len devolve o número de termos cadastrados.
func (s *FlatSynonymStorage) Len() int {
	if s == nil {
		return 0
	}
	return len(s.Terms)
}
