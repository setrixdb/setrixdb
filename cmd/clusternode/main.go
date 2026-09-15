// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Command clusternode — sobe um nó de shard do SetrixDB que escuta por TCP.
// O coordenador (clusterdemo -nodes ...) envia o shard (opLoad) e as consultas.
//
// Uso:
//
//	go run ./cmd/clusternode -addr 0.0.0.0:19100
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/setrixdb/setrixdb/internal/cluster"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:19100", "endereço de escuta")
	flag.Parse()

	nd := cluster.NewNode(*addr)
	got, err := nd.Listen()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro ao escutar:", err)
		os.Exit(1)
	}
	fmt.Println("SetrixDB clusternode escutando em", got)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	nd.Close()
	fmt.Println("encerrado")
}
