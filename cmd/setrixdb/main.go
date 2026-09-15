// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

// Command setrixdb — CLI do SetrixDB (motor de conjuntos).
//
// Uso:
//
//	setrixdb version
//	setrixdb build   -o catalogo.sxset -input produtos.txt
//	setrixdb info     catalogo.sxset
//	setrixdb has      catalogo.sxset 12345 67890
//	setrixdb intersect a.sxset b.sxset [c.sxset ...] [--list] [--bench 1000]
//	setrixdb bench   -n 1000000
//
// Formato de arquivo .sxset (binário, little-endian):
//
//	magic "SXSET1" (6 bytes) · type 'S' (esparso) · n uint64 · n × uint64
//
// O input de `build` aceita um ID `uint64` por linha, ou separados por vírgula/espaço.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/setrixdb/setrixdb"
	"github.com/setrixdb/setrixdb/internal/addb"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "version", "-v", "--version":
		fmt.Println("setrixdb 0.1.0 (engine: esparso)")
	case "build":
		err = cmdBuild(args)
	case "info":
		err = cmdInfo(args)
	case "has":
		err = cmdHas(args)
	case "intersect":
		err = cmdIntersect(args)
	case "bench":
		err = cmdBench(args)
	case "id":
		err = cmdID(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`SetrixDB — motor de conjuntos (CLI)

  setrixdb build   -o OUT.sxset -input FILE
  setrixdb info     FILE.sxset
  setrixdb has      FILE.sxset ID [ID ...]
  setrixdb intersect A.sxset B.sxset [C.sxset ...] [--list] [--bench N]
  setrixdb bench   [-n 1000000]
  setrixdb id       "termo ou frase" ["outro ..."]

Input de build: um ID uint64 por linha (ou separado por vírgula/espaço).
`)
}

// ---------- I/O de conjuntos ----------

func readIDs(path string) ([]uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ids []uint64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, tok := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == '\t' || r == ' '
		}) {
			if tok == "" {
				continue
			}
			v, err := strconv.ParseUint(tok, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ID inválido %q: %w", tok, err)
			}
			ids = append(ids, v)
		}
	}
	return ids, sc.Err()
}

func normalize(ids []uint64) []uint64 {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	w := 0
	for i := range ids {
		if w == 0 || ids[i] != ids[w-1] {
			ids[w] = ids[i]
			w++
		}
	}
	return ids[:w]
}

func writeSet(path string, ids []uint64) error {
	return setrixdb.NewSet(ids...).WriteFile(path)
}

func readSet(path string) ([]uint64, error) {
	s, err := setrixdb.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return s.IDs(), nil
}

func humanBytes(b int64) string {
	const u = 1000
	if b < u {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

// ---------- comandos ----------

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("o", "", "arquivo de saída .sxset")
	in := fs.String("input", "", "arquivo de entrada com IDs")
	fs.Parse(args)
	if *out == "" || *in == "" {
		return fmt.Errorf("use: setrixdb build -o OUT.sxset -input FILE")
	}
	t := time.Now()
	ids, err := readIDs(*in)
	if err != nil {
		return err
	}
	raw := len(ids)
	if err := writeSet(*out, ids); err != nil {
		return err
	}
	saved, _ := readSet(*out)
	fmt.Printf("construído %s\n", *out)
	fmt.Printf("  IDs lidos:      %d\n", raw)
	fmt.Printf("  IDs únicos:     %d\n", len(saved))
	fmt.Printf("  memória (n×8):  %s\n", humanBytes(int64(len(saved))*8))
	fmt.Printf("  tempo:          %s\n", time.Since(t).Round(time.Microsecond))
	return nil
}

func cmdInfo(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("use: setrixdb info FILE.sxset")
	}
	ids, err := readSet(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("arquivo:  %s\n", args[0])
	fmt.Printf("  tipo:     esparso\n")
	fmt.Printf("  membros:  %d\n", len(ids))
	fmt.Printf("  memória:  %s\n", humanBytes(int64(len(ids))*8))
	if len(ids) > 0 {
		fmt.Printf("  min/max:  %d / %d\n", ids[0], ids[len(ids)-1])
	}
	return nil
}

func cmdHas(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("use: setrixdb has FILE.sxset ID [ID ...]")
	}
	ids, err := readSet(args[0])
	if err != nil {
		return err
	}
	t := time.Now()
	for _, a := range args[1:] {
		v, err := strconv.ParseUint(a, 10, 64)
		if err != nil {
			return fmt.Errorf("ID inválido %q", a)
		}
		res := "NÃO"
		if addb.ContainsSorted(ids, v) {
			res = "SIM"
		}
		fmt.Printf("  %d -> %s\n", v, res)
	}
	fmt.Printf("(%d consultas em %s)\n", len(args)-1, time.Since(t).Round(time.Nanosecond))
	return nil
}

func cmdIntersect(args []string) error {
	fs := flag.NewFlagSet("intersect", flag.ExitOnError)
	list := fs.Bool("list", false, "imprimir os IDs do resultado")
	bench := fs.Int("bench", 0, "repetir N vezes para medir latência")
	// flags podem vir em qualquer posição: separa-as dos arquivos
	var flags, files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--bench" || a == "-bench":
			flags = append(flags, a)
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
		case strings.HasPrefix(a, "-"):
			flags = append(flags, a)
		default:
			files = append(files, a)
		}
	}
	fs.Parse(flags)
	files = append(files, fs.Args()...)
	if len(files) < 2 {
		return fmt.Errorf("use: setrixdb intersect A.sxset B.sxset [--list] [--bench N]")
	}
	sets := make([][]uint64, len(files))
	for i, f := range files {
		s, err := readSet(f)
		if err != nil {
			return err
		}
		sets[i] = s
	}
	t := time.Now()
	res := addb.IntersectMany(sets...)
	dur := time.Since(t)
	fmt.Printf("interseção de %d conjuntos\n", len(files))
	for i, f := range files {
		fmt.Printf("  %-28s %d IDs\n", f, len(sets[i]))
	}
	fmt.Printf("  => resultado: %d IDs\n", len(res))
	if *list {
		for _, id := range res {
			fmt.Println(id)
		}
	}
	fmt.Printf("  1ª execução: %s\n", dur.Round(time.Microsecond))
	if *bench > 0 {
		var best time.Duration = 1<<62 - 1
		for i := 0; i < *bench; i++ {
			t0 := time.Now()
			_ = addb.IntersectMany(sets...)
			if d := time.Since(t0); d < best {
				best = d
			}
		}
		fmt.Printf("  melhor de %d: %s\n", *bench, best.Round(time.Nanosecond))
	}
	return nil
}

func cmdID(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use: setrixdb id \"termo ou frase\" [...]")
	}
	for _, t := range args {
		id := addb.ComputeDeterministicID(t)
		fmt.Printf("  %-30q -> %d\n", t, id)
	}
	fmt.Println("\n(o ID é o mesmo para a mesma string — palavras e frases com espaço são tratadas igual)")
	return nil
}

func cmdBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	n := fs.Int("n", 1_000_000, "tamanho de cada conjunto")
	fs.Parse(args)
	r := rand.New(rand.NewSource(42))
	a := make([]uint64, *n)
	b := make([]uint64, *n)
	for i := 0; i < *n; i++ {
		a[i] = uint64(r.Int63n(int64(*n) * 4))
		b[i] = uint64(r.Int63n(int64(*n) * 4))
	}
	a = normalize(a)
	b = normalize(b)

	t := time.Now()
	res := addb.IntersectMany(a, b)
	d := time.Since(t)
	fmt.Printf("SetrixDB — bench rápido\n")
	fmt.Printf("  A: %d IDs · B: %d IDs\n", len(a), len(b))
	fmt.Printf("  |A ∩ B| = %d\n", len(res))
	fmt.Printf("  interseção: %s\n", d.Round(time.Microsecond))
	return nil
}
