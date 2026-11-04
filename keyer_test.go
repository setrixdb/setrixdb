// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package setrixdb

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/setrixdb/setrixdb/internal/addb"
)

// gerarTermosCurtos produz um vocabulário de tokens curtos — o pior caso do
// hash posicional (medido: 78% de colisão em 200 mil tokens curtos).
func gerarTermosCurtos(n int, seed int64) []string {
	r := rand.New(rand.NewSource(seed))
	// Alfabeto misto (dígitos, maiúsculas, minúsculas, espaço e hífen): é o que
	// faz o hash posicional colidir, porque o "dígito" rune+pos cansa de passar
	// da base 31 e vaza para a casa seguinte.
	alfabeto := []rune("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz -")
	out := make([]string, 0, n)
	vistos := make(map[string]struct{}, n)
	for len(out) < n {
		tam := 1 + r.Intn(8)
		b := make([]rune, tam)
		for i := range b {
			b[i] = alfabeto[r.Intn(len(alfabeto))]
		}
		t := string(b)
		if _, dup := vistos[t]; dup {
			continue
		}
		vistos[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// TestKeyerInjetividade é o teste de regressão do achado "keygen não injetivo":
// o hash posicional colide, o Keyer não.
func TestKeyerInjetividade(t *testing.T) {
	termos := gerarTermosCurtos(200_000, 42)

	// Prova de que o problema existe no hash puro.
	colisoesHash := 0
	vistos := make(map[uint64]struct{}, len(termos))
	for _, t := range termos {
		id := addb.ComputeDeterministicID(t)
		if _, dup := vistos[id]; dup {
			colisoesHash++
			continue
		}
		vistos[id] = struct{}{}
	}
	if colisoesHash == 0 {
		t.Fatalf("cenário inválido: esperava colisões no hash posicional")
	}
	t.Logf("hash posicional: %d/%d termos colidindo (%.1f%%)", colisoesHash, len(termos),
		100*float64(colisoesHash)/float64(len(termos)))

	k, err := BuildKeyer(termos)
	if err != nil {
		t.Fatalf("BuildKeyer: %v", err)
	}
	if k.Len() != len(termos) {
		t.Fatalf("Len = %d, esperado %d", k.Len(), len(termos))
	}
	t.Logf("keyer: %d termos · %d desambiguações · %.2f bits/termo",
		k.Len(), k.Colisoes(), k.BitsPerKey())

	ids := make(map[uint64]struct{}, len(termos))
	for _, termo := range termos {
		id, ok := k.Lookup(termo)
		if !ok {
			t.Fatalf("termo indexado não encontrado: %q", termo)
		}
		if _, dup := ids[id]; dup {
			t.Fatalf("ID repetido para termos distintos: %d (termo %q)", id, termo)
		}
		ids[id] = struct{}{}

		volta, ok := k.Term(id)
		if !ok || volta != termo {
			t.Fatalf("Term(%d) = %q,%v; esperado %q", id, volta, ok, termo)
		}
	}
	if len(ids) != len(termos) {
		t.Fatalf("IDs únicos = %d, esperado %d", len(ids), len(termos))
	}
}

// TestKeyerAusenteSemFalsoPositivo garante que termo fora do índice devolve false
// mesmo quando o MPHF aponta para um slot ocupado.
func TestKeyerAusenteSemFalsoPositivo(t *testing.T) {
	termos := gerarTermosCurtos(50_000, 7)
	k, err := BuildKeyer(termos)
	if err != nil {
		t.Fatalf("BuildKeyer: %v", err)
	}

	ausentes := gerarTermosCurtos(20_000, 99)
	noIndice := make(map[string]struct{}, len(termos))
	for _, t := range termos {
		noIndice[t] = struct{}{}
	}
	testados := 0
	for _, termo := range ausentes {
		if _, esta := noIndice[termo]; esta {
			continue
		}
		testados++
		if id, ok := k.Lookup(termo); ok {
			t.Fatalf("falso-positivo: %q devolveu id %d", termo, id)
		}
	}
	if testados == 0 {
		t.Fatal("nenhum termo ausente testado")
	}
	t.Logf("termos ausentes conferidos: %d · 0 falso-positivo", testados)
}

// TestKeyerDeduplicaEntrada confirma que termos repetidos não geram IDs extra.
func TestKeyerDeduplicaEntrada(t *testing.T) {
	k, err := BuildKeyer([]string{"casa", "saca", "casa", "lar", "lar", "lar"})
	if err != nil {
		t.Fatalf("BuildKeyer: %v", err)
	}
	if k.Len() != 3 {
		t.Fatalf("Len = %d, esperado 3", k.Len())
	}
	for _, termo := range []string{"casa", "saca", "lar"} {
		if _, ok := k.Lookup(termo); !ok {
			t.Fatalf("termo %q não indexado", termo)
		}
	}
}

// TestKeyerVazio cobre o vocabulário vazio (nada indexado, nada encontrado).
func TestKeyerVazio(t *testing.T) {
	k, err := BuildKeyer(nil)
	if err != nil {
		t.Fatalf("BuildKeyer: %v", err)
	}
	if k.Len() != 0 || k.Bytes() != 0 || k.BitsPerKey() != 0 {
		t.Fatalf("keyer vazio com estatísticas não nulas: %+v", k)
	}
	if _, ok := k.Lookup("qualquer"); ok {
		t.Fatal("keyer vazio encontrou termo")
	}
}

// TestKeyerIDSet cobre a ponte termos → Set de IDs (uso no motor de conjuntos).
func TestKeyerIDSet(t *testing.T) {
	k, err := BuildKeyer([]string{"casa", "saca", "lar", "moradia"})
	if err != nil {
		t.Fatalf("BuildKeyer: %v", err)
	}
	set, faltando := k.IDSet("casa", "lar", "inexistente")
	if len(faltando) != 1 || faltando[0] != "inexistente" {
		t.Fatalf("faltando = %v, esperado [inexistente]", faltando)
	}
	if set.Len() != 2 {
		t.Fatalf("set.Len() = %d, esperado 2", set.Len())
	}
	idCasa, _ := k.Lookup("casa")
	if !set.Has(idCasa) {
		t.Fatalf("set não contém o ID de %q", "casa")
	}
}

// TestKeyerAnagramasGaranteDistincao mantém a promessa do hash posicional
// (anagramas distintos) agora com colisão zero por construção.
func TestKeyerAnagramasGaranteDistincao(t *testing.T) {
	anagramas := []string{"casa", "saca", "asa", "caas", "saca "}
	k, err := BuildKeyer(anagramas)
	if err != nil {
		t.Fatalf("BuildKeyer: %v", err)
	}
	vistos := map[uint64]string{}
	for _, a := range anagramas {
		id, ok := k.Lookup(a)
		if !ok {
			t.Fatalf("%q não indexado", a)
		}
		if outro, dup := vistos[id]; dup {
			t.Fatalf("%q e %q receberam o mesmo ID %d", outro, a, id)
		}
		vistos[id] = a
	}
}

// ExampleBuildKeyer documenta o uso pretendido: indexar vocabulário uma vez e
// consultar em O(1) (substitui o uso direto de ComputeDeterministicID).
func ExampleBuildKeyer() {
	k, err := BuildKeyer([]string{"casa", "saca", "lar"})
	fmt.Println(err == nil)
	id, ok := k.Lookup("lar")
	fmt.Println(ok)
	fmt.Println(id < uint64(k.Len()))

	_, existe := k.Lookup("castelo")
	fmt.Println(existe)
	// Output:
	// true
	// true
	// true
	// false
}
