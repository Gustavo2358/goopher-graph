# Servidor residente gRPC V1

`cmd/gophergraph-server` adapta o Graph imutável, queries nativas, sandbox WASM e
GGPB existentes. Não há mutations, upload, linguagem de query, registry dinâmico,
cache de resultados ou hot-swap. Core não importa gRPC, servidor ou Linux policy.

```text
startup: mmap → validação → prefault + SHA → mlock → compile/prepare WASM → listen
                                         ↓
                              snapshot único residente
                                         ↓
RPC + deadline → admission try → query.* / registry → Runtime.ExecuteReport
                                         ↓
                              Subgraph materializado
                                         ↓
                         EmitEncoded → clone → Send → próximo batch
                                         ↓
                              Go / Python / Java
```

## Startup e residency

```sh
CGO_ENABLED=0 go build -o bin/gophergraph-server ./cmd/gophergraph-server
bin/gophergraph-server --snapshot=/data/graph.snapshot
```

Linux/amd64, arquivo regular e inode imutável. Default `locked`: prefault
integral concluído **e** sucesso de mlock no mapping inteiro. Falha encerra
startup, sem downgrade. `warm` exige prefault concluído, mas permite reclaim
posterior. `lazy` não estabelece contrato de aquecimento/locking. Validação e
identidade também leem bytes em lazy; não promete cache frio/startup sem I/O.

MADV_POPULATE_READ reporta falhas de paging em chunks de 4 MiB. Linux anterior
à introdução da API (5.14): apenas EINVAL/ENOSYS permitem fallback de leitura.
SHA-256 lê todos os bytes de cada chunk na mesma passagem de preparação; não
há terceira varredura só para identidade. Leitura integral também toca todas
as páginas no fallback. Cancelamento é checado entre chunks, sem promessa de
interromper imediatamente um syscall/fault. MAP_POPULATE/WILLNEED, mlockall e
MLOCK_ONFAULT não são usados como prova de readiness.

LockedBytes reporta bytes virtuais arredondados a páginas, separado de Size.
RLIMIT_MEMLOCK precisa acomodar tamanho arredondado a páginas e outros locks
no processo. CAP_IPC_LOCK pode contornar o limite, mas não protege de cgroup
OOM. Containers precisam de memlock suficiente, memória para snapshot + heap
+ compiled code + transporte e seccomp permitindo essas operações. Não é
necessário rodar como root. GOMEMLIMIT não inclui mmap nem todo código nativo.

`/proc/self/smaps`, RSS/PSS/Locked PSS são diagnóstico opcional: sua restrição
não impede READY. Locked PSS não equivale aos bytes estabelecidos por mlock.
Snapshot inexistente/inválido, erro prefault/lock ou falha WASM impedem startup
antes do listener. MAP_PRIVATE não protege de truncate externo/SIGBUS. Publicar
snapshot novo por rename; V1 continua usando a geração que abriu.

## Contrato remoto e ownership

[Schema remoto](../remote/pb/service.proto): gophergraph.remote.v1.
[Schema GGPB](../ggpb/pb/result.proto): gophergraph.ggpb.v1, separado.
Territory, AntiTerritory, Between e RunWasm são server-streaming;
ListWasmQueries/GetServerInfo são unary. IDs externos optional distinguem
campo obrigatório ausente de ID vazio válido. Filtro ausente = todas as labels;
presente vazio = nenhuma. Labels desconhecidas não ampliam o filtro.

Territory/AntiTerritory incluem origem; Between intersecta alcances forward e
reverse, incluindo passeios com ciclos, conforme biblioteca atual.
EncodedBatch.ggpb contém um Batch Protobuf GGPB sem magic/length LE/CRC de
arquivo. Header → Records* → End. **Completo exige End validado e status final
OK**; erros invalidam todo prefixo. Não retomar por página. Metadata inclui
snapshot-id, ggpb-version e, para WASM, wasm-query/wasm-sha256. SHA identifica
bytes exatos, não autentica sua origem.

EmitEncoded empresta bytes até callback retornar. Adapter faz clone para
ownership imutável; marshaler Protobuf padrão copia no wrapper: **duas cópias
adicionais de payload Go**, mais buffers/cópias de transporte/kernel/TLS/decode.
Não é zero-copy. Message não é mutado após Send. Nenhum codec proprietário,
channel produtor ou paralelismo interno. Send síncrono pode bloquear por
HTTP2 write quota/flow control antes de avançar ao próximo batch.

Subgraph permanece materializado e seus bitsets dependem do snapshot. Bounded
memory significa admission finito, workspaces/batches bounded e limites WASM/
transporte, não memória constante para qualquer grafo ou recuperação de OOM.
Consumidores de referência retêm um batch; caches que consumidores criarem
não estão sob controle do servidor.

## Admission e configuração

Token global desde antes do decode até fim de envio/cleanup. Try sem fila:
RESOURCE_EXHAUSTED. Native/WASM compartilham orçamento. Ordem: global try →
WASM runtime try; falha libera global. Slot WASM termina com guest; token global
permanece enquanto Subgraph espera rede. Sem fila dupla, thread pool próprio,
workers internos por query ou promessa FIFO por cliente.

Queries exigem deadline **do cliente** dentro do máximo do servidor; ele
cancela o transporte/Send. Context derivado do handler sozinho não interrompe
Send bloqueado. Timeout WASM é o menor entre seu limite e deadline da query.

| Opção | Default / significado |
|---|---|
| --listen | 127.0.0.1:9090 |
| --metrics-listen | 127.0.0.1:9091; vazio desabilita |
| --residency | locked |
| --query-capacity | 0: min(GOMAXPROCS, orçamento/envelope estimado) |
| --query-memory-budget | 512 MiB; exclui snapshot/runtime base |
| --max-query-deadline | 1 minuto; deadline do cliente obrigatório |
| --shutdown-grace / --startup-timeout | 30 s / 5 min |
| --max-connections | 128; global, antes de TLS/HTTP2 |
| --max-streams-per-connection | 64; não substitui admission |
| --max-request-bytes | 256 KiB |
| --wasm-concurrency | 4; orçamento adicional, sem fila |
| --wasm-memory-pages / --wasm-host-bytes | 1024 (64 MiB) / 64 MiB |
| --wasm-handles / --wasm-calls | 256 / 10000 |
| --wasm-args-bytes | 65536 incluindo terminadores |
| --wasm-timeout / --wasm-compile-timeout | 5 s / 30 s |
| --tls-cert / --tls-key | PEM; juntos habilitam TLS >=1.2 |
| --allow-insecure | false; plaintext fora de loopback exige autorização via flag |

Envelope estima BFS/bitmaps, guest+host WASM e 6 × MaxFrame (24 MiB) para encoder/transporte.
É heurística conservadora, não hard cap de RSS. Override acima do orçamento
estimado é rejeitado. Calibrar com workload real: CPUs, GC, resultados, guests
e consumidores variam. Reservar memória adicional para base, conexões e
compiled code/cache. Limites anteriores de módulo/cache/table permanecem ativos.

Headers 8 KiB, handshake 10 s. Max resultado = MaxFrame GGPB + 16 bytes de
wrapper. Configurar max receive no cliente: default gRPC 4 MiB pode rejeitar
maior wrapper. Janelas estáticas servidor: 64 KiB stream, 1 MiB conexão, sem
BDP auto-grow; peer também participa do flow control. Compressão não habilitada.

## WASM instalado

[Registry](../remote/installedwasm/registry.go) imutável, ordenado, nomes ASCII
únicos. Inclui shared-targets(2 args), between(2), filtered(4): descrição,
parâmetros nominais, ABI, versão e SHA original. Identidade do resultado:
`wasm:<name>@<sha256 completo>`. ExpectedSha256 diferente: FAILED_PRECONDITION.
Registry tem digest estável; discovery retorna cópias, sem aliases mutáveis.

Assets WASI embutidos e versionados. Atualizar no build/deployment, nunca por
RPC: `go generate ./remote/installedwasm`. Go WASI, trimpath/buildid vazio,
sem metadata VCS que mudaria o hash a cada commit;
reproduzir exige mesma versão Go e fontes SDK. Startup compila/cacheia, resolve
names/signatures de imports contra exports reais, instancia sem `_start`.
Binary start section, se existir, roda bounded sem Graph. Nenhuma query falsa
para health/first-request compilation. Todos os módulos são obrigatórios V1.

Guest fresco por execução; runtime reutilizável. Sem filesystem/socket/env/
stdio/Go pointers/snapshot bytes/capabilities de context ambiente. Memória,
table, handles, calls, host bytes e timeouts limitados. JSON bounded dentro
ABI host↔guest preservado; protocolo remoto/resultados são Protobuf binário.

## Health, erros e shutdown

Health gRPC padrão Check/Watch: serviço vazio e GraphService. READY exige
snapshot validado/preparado, contrato residency estabelecido, runtime/registry
compilado/linkado e serving. Saturação não remove readiness. Startup não pronto
recusa conexão. Liveness não consulta grafo. Draining: NOT_SERVING, sem admission.

SIGTERM: seal admission/handlers → NOT_SERVING → cancel Watch → aguardar graça
→ se expirar, cancel lifecycle + Stop → join leitores → fechar/join scrapes
HTTP → Runtime.Close → Graph.Close (Munlock/Munmap). Nenhum Close durante leitura.

Não usar GracefulStop em background seguido de Stop por timer. Reproduzido
[deadlock #9393](https://github.com/grpc/grpc-go/issues/9393) na versão 1.84.0.
GracefulStop só começa após nossos handlers terminarem. No timeout Stop ocorre
antes de qualquer GracefulStop. Watch tem cancelamento próprio. Shutdown retorna
após quiescência mesmo com graça expirada; syscall/alocação/OOM/SIGBUS não têm
promessa impossível de interrupção instantânea/recuperação.

| Falha | Resposta e ownership |
|---|---|
| snapshot/prefault/lock/WASM compile/link inválido | startup falha, libera recursos, sem READY |
| ID ausente, args inválidos, deadline ausente/excessivo | INVALID_ARGUMENT |
| ID/nome WASM desconhecido | NOT_FOUND |
| expected hash diferente | FAILED_PRECONDITION |
| admission/guest/host/request/batch oversized | RESOURCE_EXHAUSTED, sem fila |
| cliente cancela | CANCELLED; libera token/handles/guest |
| deadline/timeout WASM | DEADLINE_EXCEEDED |
| trap/erro inesperado | INTERNAL; sem dados/args na mensagem |
| draining | UNAVAILABLE para novas queries |
| rede/writer falha | status transporte preservado; descartar prefixo |
| consumidor lento/resultado grande | Send bloqueia bounded; mantém token; deadline limita duração |
| shutdown em query ativa | aguarda graça/cancela transporte/join antes de unmap |

## Observabilidade e clientes

HTTP operacional: /metrics em texto Prometheus, /livez, /readyz. Scrapes joinados
antes de unmap; nenhuma REST de query. Métricas: uptime, RSS opcional, heap,
alocações/GC/goroutines/GOMAXPROCS; snapshot ID/bytes/N/E/mode/warm/locked/prepare
+ RSS/PSS opcional; admission/requests/active/rejects/cancel/failed; histogramas
de duração; execução/encoding/Send; resultados/payload bytes/batches aceitos;
WASM compile/cache/active/handles/calls/bytes/peaks/timeouts/traps. Labels finitas:
queries instaladas + wasm:<unknown>; sem IDs/args. Peaks são máximos observados,
não counters somáveis. Bytes accepted não prova delivery, nem inclui HTTP2/TLS.
Send mede marshal/flow-control/enqueue, não rede isolada.

```sh
CGO_ENABLED=0 go build -o bin/gophergraph-remote-client ./clients/go
bin/gophergraph-remote-client --insecure --node=PROGRAM:A
python3 -m venv /tmp/gophergraph-python
/tmp/gophergraph-python/bin/pip install -r clients/python/requirements.txt
/tmp/gophergraph-python/bin/python clients/python/client.py --insecure --node=PROGRAM:A
```

Nos CLIs, `--query=wasm:between` distingue o módulo instalado do RPC nativo Between.
Python: stub.Territory(request, timeout=...), iterar batch.ggpb sem páginas.
RunWasm usa mesmo modelo. Validar End e status final. Cancelar RPC ao abandonar
consumo. Lambda pode reutilizar channel e limitar deadline ao tempo restante;
integração AWS fora desta V1. Java smoke gera mesmos protos por Maven.

Geração explícita: grpcio-tools em venv, protoc-gen-go 1.34.2 (sem unsafe próprio)
e protoc-gen-go-grpc 1.6.2 no PATH. Rodar
`PYTHON=/tmp/gophergraph-python/bin/python sh tools/generate_remote.sh`.
Nenhum download/generation escondido em testes Go. Build limpo usa assets/fontes
versionados. [Qualificação](REMOTE_BENCHMARKS.md).
