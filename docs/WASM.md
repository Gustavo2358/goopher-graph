# Queries Go em WebAssembly

`wasmquery` executa programas Go `wasip1/wasm` sobre um `graph.Graph` já aberto.
Usa [wazero](https://wazero.io/) v1.12.0, pure Go, sem cgo. A dependência exige
x/sys v0.44.0; o adapter mmap continua sendo o único importador direto de unix.
Snapshot, ingestão, queries nativas e contrato [graphjson](JSON.md) são preservados.

## Compilar e executar

Na raiz, com Go 1.26 e as dependências de `go.mod` disponíveis:

```sh
CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build \
  -o /tmp/between.wasm ./examples/wasm/between
bin/gophergraph build --nodes fixtures/01_topology/nodes \
  --edges fixtures/01_topology/edges --output /tmp/graph.snapshot
bin/gophergraph wasm --snapshot /tmp/graph.snapshot \
  --module /tmp/between.wasm --arg A --arg F --output /tmp/result.json
```

`--arg` é repetível e preserva ordem, strings vazias e Unicode. O programa recebe
`os.Args[0] == "query.wasm"` e os argumentos seguintes. NUL é rejeitado. Use
`--arg=--flag` para valores que começam com hífen. `--timeout 5s` limita a execução;
a compilação tem seu próprio limite. O CLI carrega bytes compilados, sem compilar
source Go. Sem `--output`, o JSON vai para stdout. Stderr informa falhas e snapshot
parcial. Códigos: 0 sucesso, 1 falha operacional/WASM, 2 uso inválido, 3 ID ausente.
O resultado usa `query.name = "wasm:<sha256 dos bytes originais>"`, com os demais
campos do contrato existente. Argumentos não são acrescentados ao contrato JSON.

## SDK do autor

```go
package main

import (
    "os"
    "gophergraph/wasmquery/sdk"
)

func main() {
    if len(os.Args) != 3 { os.Exit(2) }
    q := sdk.New()
    a := q.Nodes(os.Args[1]).Territory()
    b := q.Nodes(os.Args[2]).AntiTerritory()
    if err := q.Return(a.Intersection(b).Induced()); err != nil { os.Exit(1) }
}
```

O SDK esconde imports, ponteiros e mensagens da ABI. `NodeSet`, `EdgeSet` e
`Subgraph` guardam apenas uma referência à sessão SDK e um handle de 64 bits.
Operações produzem novos conjuntos; não modificam os anteriores. `q.Err()` é
sticky; `q.Return(subgraph)` retorna esse erro. Erros da Host API encerram o guest
e retornam erro ao chamador Go, mesmo se o programa ignorar o retorno do SDK.
`Return` deve ser a última operação de grafo e ocorrer exatamente uma vez.
Depois disso o programa deve terminar com sucesso; trap, exit não zero ou
cancelamento descartam o resultado.

| API | Semântica |
|---|---|
| `q.Nodes(ids...)`, `q.NodesWithLabel(label)` | IDs externos; ID inexistente é erro. `Nodes()` e label ausente produzem conjunto vazio. |
| `nodes.Out/In/Both(labels...)` | Vizinhos distintos a um salto, por direção. |
| `nodes.OutE/InE/BothE(labels...)` | Edges por identidade; paralelas preservadas, loop incluído uma vez. |
| `edges.Sources/Targets()` | Endpoints na direção original. |
| `set.HasLabel(label)`, `set.Has(key, value)` | Filtros em nodes ou edges; igualdade tipada nativa. Sets de propriedades casam se algum valor for igual. |
| `set.Union/Intersection/Difference(other)` | Mesmo tipo e sessão; deduplicação por identidade. |
| `nodes.Reachable(direction, labels...)` | BFS com múltiplas sementes; inclui sementes, expande cada node uma vez. Direções `Forward`, `Reverse`, `Bidirectional`. |
| `nodes.Territory/AntiTerritory(labels...)` | NodeSets de alcance forward/reverse. Correspondem aos nodes das queries nativas. |
| `nodes.Induced(labels...)` | Subgrafo com todas as edges permitidas entre nodes selecionados. |
| `q.Subgraph(nodes, edges)` | Seleção explícita; acrescenta os endpoints das edges. |
| `sub.Nodes/Edges()` | Novos handles dos conjuntos do subgrafo. |
| `set.Count/Empty()` | Escalares; subgrafo tem `NodeCount`, `EdgeCount`, `Empty` (sem nodes). |
| `q.NodeProperty/EdgeProperty(id, key, index)` | Um valor tipado e `found`; índice zero-based na ordem canônica da propriedade. Chave/índice ausente dá false, entidade ausente é erro. |
| `handle.Release()` | Libera antecipadamente; aliases ficam inválidos. Não usar `defer Release` depois de `Return`. |
| `q.Return(sub)` | Seleciona o resultado mantido no host. |

Labels de travessia: variádico omitido/nil permite todas; slice explicitamente
vazio permite nenhuma; desconhecido não casa. Construtores `Bool`, `Byte`,
`Short`, `Int`, `Long`, `Float`, `Double`, `String`, `Date`, `Datetime` preservam
as dez tags. Leituras retornam `Value{Kind, Bits, Text}`: inteiros em complemento
de dois, floats em IEEE 754, textos em UTF-8. NaNs são canonicalizados pelo host;
zeros assinados seguem a igualdade nativa. Não há enumeração de todos os membros
para a memória WASM; filtros, álgebra, BFS e formação de subgrafo ficam no host.

Exemplos executáveis:

- [shared targets](../examples/wasm/shared_targets/main.go): interseção dos nodes
  alcançáveis por duas origens, equivalente ao exemplo Go nativo, com edges induzidas.
- [between](../examples/wasm/between/main.go): `territory(A) ∩ antiTerritory(B)`.
- [filtered](../examples/wasm/filtered/main.go): `out`, label/property tipada, `in`
  e interseção de edges. Argumentos para a fixture: `A PROGRAM sigla CO`.

Para um módulo Go independente, use `require gophergraph v0.0.0` e
`replace gophergraph => /caminho/local/gophergraph` no `go.mod`. Copie um exemplo,
importe somente `gophergraph/wasmquery/sdk` e compile com o comando acima trocando
o package por `.`. O teste E2E faz isso offline, fora deste módulo.

## Host API v1

Import `gophergraph_v1.call`:

```text
(op i32, a i64, b i64, params_ptr i32, params_len i32,
 output_ptr i32, output_capacity i32) -> i64
```

`a`/`b` são handles, exceto `b` em Count/Empty de subgrafo (0 nodes, 1 edges).
O retorno é handle, contagem, booleano 0/1 ou número de bytes escritos para uma
leitura de property. Release/Return retornam 0. `UINT64_MAX` sinaliza falha;
o host também encerra a instância e preserva o erro tipado. Token 0 é inválido.
Ponteiros, comprimentos, assinaturas e tipos de handles são checados. Handles não
são reutilizados nem resolvidos fora da execução que os criou.

Parâmetros opcionais são um objeto JSON UTF-8 limitado, com os campos necessários
à operação: `ids`, `label`, `key`, `value`, `direction`, `labels`, `id`, `index`.
`labels:null` permite todas; `[]` nenhuma. `value` é `{kind,bits,text}` conforme
acima. Leitura pontual escreve `{found,value}`. Não há payload de NodeSet,
EdgeSet, Subgraph ou JSON de resultado nessa fronteira. A memória de parâmetros
é lida apenas durante a chamada; o host não retém slices da memória guest.

| Opcode | Operação | Opcode | Operação |
|---|---|---|---|
| 1 | Nodes | 2 | NodesLabel |
| 3 / 4 / 5 | Out / In / Both | 6 / 7 / 8 | OutE / InE / BothE |
| 9 / 10 | Sources / Targets | 11 / 12 | Label / Property |
| 13 / 14 / 15 | Union / Intersection / Difference | 16 | Reachable |
| 17 / 18 | Induced / Subgraph | 19 / 20 | SubNodes / SubEdges |
| 21 / 22 | Count / Empty | 23 / 24 | ReadNodeProperty / ReadEdgeProperty |
| 25 / 26 | Release / Return | | |

## Runtime reutilizável e ownership

O adapter chama `New(ctx, Limits{})`, `Compile(ctx, wasmBytes)` e
`Execute(ctx, module, graph, args)`. Recebe `Result`, usa `Subgraph()` com
`graphjson.Write`, chama `Result.Close()` e, ao encerrar o serviço, `Runtime.Close()`.
O runtime não lê paths, abre snapshots ou escolhe writers. Isso permite que outro
driver use a mesma capacidade. Nenhum HTTP foi implementado.

O SHA-256 identifica os bytes originais. Compilações simultâneas do mesmo conteúdo
convergem para o mesmo `Module`. Cache e runtime pertencem ao processo chamador;
um serviço mantém um runtime aberto entre chamadas. O CLI termina o processo ao
final. Cache cheio rejeita novo conteúdo; não há eviction, cache em disco ou cache
de resultados. `Module` não tem Close individual; dura até `Runtime.Close()`.

Execute admite concorrência limitada e cria memória, globals, argumentos, handles
e sets novos a cada chamada. O grafo é compartilhado somente para leitura.
`Close` cancela e aguarda as operações ativas antes de liberar módulos compilados.
Handles são removidos em sucesso, erro, trap ou cancelamento. Só o subgrafo
selecionado sobrevive à execução, sob ownership de `Result`. Seu Close solta essa
referência; não usar referências obtidas dele depois do Close. Resultados entregues
sobrevivem ao Close do runtime. O chamador mantém o Graph aberto até terminar o
uso de todos os resultados e não faz Close de Result simultâneo ao seu uso.

## Sandbox e limites conhecidos

A instância recebe apenas WASI Preview 1 e a Host API v1. WASI usa os defaults
restritivos do wazero: filesystem sem preopens, nenhuma rede/FD herdado, ambiente
vazio, stdin EOF, stdout/stderr descartados, relógios e aleatoriedade sintéticos.
Imports extras e memória importada são rejeitados. Valores de contexto externos
são isolados, inclusive opt-ins do wazero para sockets; deadline/cancelamento
são preservados. Guest não recebe bytes de
snapshot, mmap, ponteiros Go ou acesso aos packages internos. Go `unsafe` dentro
do guest continua restrito à memória linear; o SDK não precisa de unsafe.

| Limite padrão | Valor |
|---|---|
| Memória linear por instância | 64 MiB (1.024 páginas) |
| Tabela de funções | Uma funcref, até 65.536 elementos; máximo inserido quando omitido |
| Bytes do módulo / cache | 16 MiB / 128 MiB de bytes originais, até 16 módulos |
| Execuções simultâneas | 4; excedentes recebem ErrLimit |
| Handles vivos / chamadas por execução | 256 / 10.000 |
| Bitsets + fila de BFS por execução | 64 MiB, incluindo temporários contabilizados |
| Argumentos / mensagem da ABI | 64 KiB (argumentos incluem terminadores NUL) / 64 KiB |
| Texto em leitura pontual | Até 10.922 bytes UTF-8 e resposta codificada dentro de 64 KiB |
| Tempo de execução / compilação | 5 s / 30 s, ou contexto mais curto |

`Limits` permite ajustes pelo driver; campos zero selecionam defaults. O contexto
interrompe loops WASM e os loops de grafo no host. O limite de compilação usa o
cancelamento cooperativo do wazero, sem promessa de preempção de cada etapa de
validação/compilação. O perfil aceita comandos com `_start`, não reactors.
Tabelas múltiplas e tipos de tabela diferentes de funcref são rejeitados.

Os orçamentos de bitsets e bytes de entrada não são um teto de RSS: grafo/mmap,
scalars lidos do Graph, metadados, código compilado, stack do runtime, Go GC e
resultados retidos pelo chamador têm custos separados. Go OOM fatal não é
recuperado. O chamador limita resultados retidos e dimensiona o processo para
seu corpus. Não há fuel/instruction metering, execução distribuída, linguagem
própria, registry, HTTP, S3, autenticação ou compilação Go no servidor.
