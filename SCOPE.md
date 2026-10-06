# Escopo e critério de término

## Missão

Carregar um grafo de dependências em representação compacta e oferecer travessias eficientes, sem construir um banco de grafos. A engine desconhece o significado de PROGRAM, CALLS, FILE, sigla ou qualquer outro conceito de negócio.

O produto inicial é uma biblioteca Go reutilizável e uma CLI local. Cloud é uma possibilidade de novos adapters, não uma entrega antecipada.

## Incluído

**Entrada:** dois catálogos explícitos, nodes e edges, materializados localmente por diretórios. Formato Neptune Gremlin CSV, UTF-8, labels e propriedades tipadas. Todos os nodes antes das edges. Rejeição isolada de headers, registros e endpoints inválidos. Relatório completo/parcial e diagnósticos estruturados.

**Modelo:** directed property multigraph, IDs externos de nodes/edges, multilabel de nodes, um label de edge, propriedades single/set de nodes e single de edges, loops e paralelas. Canonicalização determinística, CSR forward/reverse, dicionário de strings e postings compactos. Índices de propriedade apenas para chaves escolhidas.

**Persistência:** layout binário desta spec, validação rigorosa, snapshot imutável, publicação local com staging e rename. Leitura por mmap em Linux/amd64. Codec testável em memória. Arquivo maior que 4 GiB não é proibido pelo layout, sujeito aos limites efetivos de memória/arquitetura.

**Uso:** território, antiterritório, região entre dois nodes; filtro de labels de edge; lookup de metadados e igualdade de propriedades; bitsets e iteração para consultas Go próprias. Saída CSV de IDs, DOT ou JSON estruturado por streaming, sem integração com renderer.

**Engenharia:** vertical slices, interfaces mínimas nas fronteiras, testes unitários/E2E, fuzz direcionado, race detector, benchmarks locais e documentação de uso real. O produto compila sem cgo.

## Excluído

HTTP/REST, S3 implementado, SDK AWS, IAM, autenticação, execução distribuída, daemon, hot reload, plugins dinâmicos, DSL, Gremlin/Cypher, planner, optimizer, transações de graph, mutações online, ingestão incremental, compactação, WAL, storage remoto por página, GUI, Graphviz embarcado, cache de respostas, pool global, paralelismo interno de BFS, engine ponderada e enumeração de caminhos simples.

Nenhuma tarefa de escrever coleções básicas da stdlib. Nenhuma obrigação de `unsafe`, SIMD, mmap zero-cost ou micro-otimização antes de medir. Não há obrigação de API C/FFI, biblioteca `.so` ou wrapper Java.

## Limites explícitos

Compatibilidade é com o formato Gremlin CSV no perfil detalhado em INGEST, não equivalência integral com comportamento transacional do serviço Neptune. Datas ficam como texto validado e valores tipados preservam as regras locais; divergências deliberadas estão documentadas.

Bad data pode produzir grafo vazio/parcial, mas nunca dangling edges. Falhas globais de catálogo, recursos, cancelamento, diagnóstico ou publicação são operacionais. OOM fatal do runtime não é recuperável como um registro inválido.

O snapshot não autentica a origem nem tolera alteração externa dos bytes enquanto mapeado. O chamador encerra consultas antes de Close. `-race` não é prova de segurança contra unmap externo ou SIGBUS.

## Produto concluído

As 13 fatias estão concluídas por evidência: diretórios reais -> carga resiliente -> snapshot -> mmap -> três consultas corretas; propriedades/labels/identidades preservadas; índices e scan concordam; rejeições não abortam o lote; corrupção é rejeitada; publicação e fechamento de recursos são testados; consulta externa usa somente API pública; fuzz/race/bench foram executados e o README reproduz uso real.

Não há meta de linhas de código, marcação de release ou SLA inventado. Um arquivo real do usuário pode qualificar o ambiente depois; não se declara testado um corpus que não foi disponibilizado.

## Extensão solicitada: queries WASM

Programas Go externos compilados para `wasip1/wasm` podem compor operações de
grafo por uma Host API pública de handles. `wasmquery` usa wazero pure Go, mantém
cache local de módulos compilados e isola cada execução. O CLI aceita snapshot,
WASM e argumentos; retorna o contrato graphjson existente. O SDK e os limites
estão em [WASM](docs/WASM.md). Essa extensão não altera o snapshot nem as queries
nativas. HTTP, registry, S3, autenticação e compilação de source no servidor
continuam excluídos.

## Extensão solicitada: resultados GGPB

Adapter Protobuf lógico e versionado, externo ao core; query completa → Subgraph
→ batches limitados → framing de arquivo. CLI das três queries nativas, WASM e decoder
incremental para o contrato JSON. [Contrato](docs/GGPB.md) e
[medições](docs/GGPB_BENCHMARKS.md). Runtime oficial Protobuf autorizado por essa
extensão. Dicionários locais não alteram layout de snapshot nem execução da query.
API gRPC, HTTP, Lambda e streaming durante traversal permanecem excluídos.
