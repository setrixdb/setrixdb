# SetrixDB distribuído em Kubernetes — 3 nós (referência)

Manifesto **genérico** para subir 3 nós de shard do SetrixDB (`cmd/clusternode`) mais o
coordenador (`cmd/clusterdemo`). É o mesmo arranjo usado na validação de cluster (2^24 → 2^30).

## Topologia

- **Estrela**: o **coordenador** abre 1 conexão TCP por nó (`cmd/clusterdemo -nodes`).
  Os nós **não falam entre si**.
- Porta: **TCP 19100** (1 por nó; `-addr` sobrescreve). Sem HTTP, sem TLS, sem multicast,
  sem porta privilegiada.
- Só é preciso **coordenador → nó :19100** (pod↔pod no mesmo namespace basta).

## Passos

### 1. (Opcional) construir os binários estáticos

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o clusternode ./cmd/clusternode
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o clusterdemo ./cmd/clusterdemo
```

### 2. Subir os nós

```bash
kubectl apply -f deploy/k8s/statefulset.yaml
kubectl -n setrixdb rollout status statefulset/sxnode
kubectl -n setrixdb get pods -o wide     # 1 pod por nó (anti-affinity)
```

### 3. Copiar o binário e iniciar o nó em cada pod

```bash
for i in 0 1 2; do
  kubectl -n setrixdb cp ./clusternode sxnode-$i:/tmp/clusternode
  kubectl -n setrixdb exec sxnode-$i -- \
    sh -c 'chmod +x /tmp/clusternode; setsid /tmp/clusternode -addr 0.0.0.0:19100 </dev/null >/tmp/cn.log 2>&1 & sleep 1; cat /tmp/cn.log'
done
```

### 4. Rodar o coordenador (a partir do pod 0)

```bash
kubectl -n setrixdb cp ./clusterdemo sxnode-0:/tmp/clusterdemo
IPS=$(kubectl -n setrixdb get pods -l app=sxnode -o jsonpath='{range .items[*]}{.status.podIP}:19100,{end}' | sed 's/,$//')
kubectl -n setrixdb exec sxnode-0 -- /tmp/clusterdemo -nodes "$IPS" -universe 16777216 -q 100
```

`-universe` é o universo em **bits** (2²⁴ = 16777216); `-q` é o nº de repetições por modo.

## Notas / limites medidos

- **Banda:** o modo **ad-hoc** transmite a fatia da consulta **a cada consulta** (~O(|Q|/S) por nó).
  O modo **armazenado** (`A ∩ B` de conjuntos já carregados) trafega **só os nomes** — zero dados por
  consulta. Em `2^28`, o ad-hoc movimentou ~3 GB intra-cluster com `-q 50`.
- **Memória do coordenador:** `clusterdemo` materializa os conjuntos **globalmente** antes de fatiar
  (`LoadSet(global[lo:hi])`) e ainda aloca as consultas do teste (~10·W palavras no total). Com
  `GOGC=100` (~2× do *live*), **~1 GiB** estoura por volta de **2^29**. Para universos maiores, o
  coordenador é o gargalo — melhoria prevista: fatiar por *seed*/streaming em vez de materializar o
  global (+ `GOMEMLIMIT`/`GOGC` no agente).
- **Limpeza:** `kubectl delete -n setrixdb statefulset/sxnode svc/sxnode` (comandos separados: o
  `kubectl` interpreta `delete A x svc y` como nomes).
