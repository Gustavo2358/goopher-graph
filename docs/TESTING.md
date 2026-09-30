# Protocolo de testes

## Regra de execução

Cada fatia fecha comportamento observável com testes reais. O estado é registrado apenas em PROGRESS. Não criar targets que imprimem PASS sem testes; um package inexistente não está aprovado. Referências Python e snippets de API não são testes da engine.

Testes unitários/integrados ficam em `_test.go` perto da capacidade. E2E em `tests/e2e` executa o binário real em diretórios temporários. Usar `testing`, `t.TempDir`, `t.Cleanup`, table tests e `os/exec`; sem framework externo.

## Comandos

A partir de B00: `go test ./...`, `go vet ./...` e conferência de `gofmt -l` nas fontes reais. Testes por capacidade, como `go test ./graph ./query`, passam a existir quando esses packages forem implementados. Não adicionar packages vazios para satisfazer comandos de uma fatia futura.

Build de produto: `CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph`.

Race: `CGO_ENABLED=1 go test -race ./...` em Linux/amd64 com suporte de compilador para a instrumentação. Não executar com CGO_ENABLED=0 e chamar a falha de ausência de races. [G5](REFERENCES.md)

Fuzz em B10, além das seeds que rodam em `go test`:

```sh
go test ./ingest/adapters/neptune -run '^$' -fuzz '^FuzzNeptuneCSV$' -fuzztime=10000x
go test ./snapshot -run '^$' -fuzz '^FuzzSnapshotDecode$' -fuzztime=10000x
```

O objetivo é executar ao menos essas campanhas finitas com seeds úteis, não provar correção por um contador arbitrário. Bugs encontrados viram casos mínimos persistidos. Fuzz é nativo do Go; não adicionar outro motor. [G10](REFERENCES.md)

Benchmarks: `go test -run '^$' -bench . -benchmem ./...`. No fechamento: `go clean -testcache`, seguido de `go test -count=1 ./...`, vet, race e build sem cgo. Não limpar caches globais de dependências nem exigir rede para simular diretório limpo.

## Matriz mínima

| IDs | Evidência exigida |
|---|---|
| G01–G03 | Vazio/isolado, direção, loops, paralelas, IDs válidos/inválidos e ranges |
| G04–G05 | Multilabel, todos os tipos, igualdade canônica, NodeSet/EdgeSet e identidade do Graph |
| G06 | API não expõe alias gravável nem strings unsafe sobre mmap; Close sequencial idempotente |
| Q01–Q03 | Território/antiterritório reflexivos, cadeia/diamante/ciclo, quatro edges no diamante |
| Q04 | Between, unreachable, A=B, passeio com ciclo que não é caminho simples |
| Q05 | Filtro nil=todas, slice vazio=nenhuma, label desconhecido, paralelas distintas |
| Q06 | Forward/reverse preservam source/target lógico; DOT não inverte edges |
| Q07 | Oráculo independente de closure em pequenos grafos aleatórios |
| Q08 | Cancelamento em query grande, inclusive hub; nenhum resultado incompleto como sucesso |
| C01–C03 | Quotes, escaped quotes, vírgulas, Unicode, multiline, LF/CRLF e EOF sem newline |
| C04 | Ausente versus quoted vazio; ID vazio e propriedade vazia preservados |
| C05 | CRLF dentro de string preservado, espaços externos tratados sem apagar espaços quoted |
| C06 | Headers de papel errado, repetidos, propriedade duplicada, tipos/arrays/cardinalidade inválidos |
| C07 | Todas as tags, limites numéricos, Bool coerced, NaN/Inf, INF/hex recusados, datas |
| C08 | Aridade/valor/UTF-8 inválidos entre válidos: rejeitar apenas record delimitado |
| C09 | Aspas abertas até EOF: prefixo conservado e próxima fonte carregada |
| C10 | Fragmentação 1/2/7/4096 bytes; n>0 com EOF/erro; record maior que 64 KiB não quebra por Scanner |
| C11 | Limite de record antes de crescer sem controle; drain só com fronteira confiável |
| I01–I03 | Nodes/edges separados, barreira real e endpoint ausente nunca vira placeholder |
| I04–I06 | Merge idempotente, conflito single/remove propriedade, conflito de EdgeID/quarentena |
| I07 | Fonte I/O falha sem matar lote; catálogo raiz falho é fatal |
| I08 | Empty completo, all-rejected parcial e header inválido isolado |
| I09 | Contadores disjuntos, staged != entidades finais, warnings != perdas, logs uma vez |
| I10 | Erro no DiagnosticSink é operacional; propriedades sensíveis não vazam no diagnóstico |
| I11 | Permutações de records válidos/arquivos/opções conservam bytes e conflitos |
| I12 | Cancelamento de ingestão não publica prefixo como carga parcial bem sucedida |
| X01 | Índice e scan concordam para nodes/edges, todos os tipos, chave ausente/índice vazio |
| X02 | Postings crescentes/únicos e cobertura de todos os memberships |
| S01–S02 | Writer produz goldens exatos; reader abre goldens; round-trip heap/bytes/mmap |
| S03–S04 | Truncamento, magic, flags, stride, count, overflow, offsets, padding e reserved inválidos |
| S05 | CSR ou postings com membership falso/faltante/duplicado são rejeitados |
| S06 | Falha de acquire/decode fecha backing uma vez; Close libera mapping sem finalizer |
| S07 | Seal/validação falhos não chegam a Commit |
| S08 | Falhas begin/write/sync/rename antes de publicar preservam anterior |
| S09 | Falha pós-rename informa PublishedUncertain sem apagar destino ou prometer rollback |
| S10 | Leitor antigo continua correto após publicar outra geração no mesmo nome |
| S11 | Writer trata short writes e erro de flush/Close; não duplica arquivo inteiro em heap |
| E01 | Diretórios reais -> snapshot real -> três comandos em subprocesso |
| E02 | Exit codes, argumentos obrigatórios, --node="" presente versus ausência |
| E03 | CSV IDs com quoted vazio/multiline; stderr separado de stdout; partial explicitado |
| E04 | DOT não-strict, escaping seguro, loops/paralelas e direção preservados |
| E05 | Consulta Go em consumidor separado usa apenas API pública sem editar core |
| E06 | Imports por capacidade; core sem adapters/I/O; produto sem cgo |
| R01 | Race detector em leituras concorrentes de um Graph aberto, sem workspace global |
| R02 | Fuzz CSV/snapshot não produz panic por bytes externos nem leitura fora de limite |
| R03 | Erros de portas e cancelamento não vazam arquivos, staging ou mappings |
| R04 | Inputs de CSV/DOT/log não viram sintaxe/caminhos executáveis |
| R05 | Open/query/Close repetidos estabilizam recursos observáveis e não crescem sem limite |
| P01–P03 | Escala, fórmula de bytes, expansão por node, fases e memória medidas |

## Oráculos e metamórficos

Fixtures possuem modelo final, consultas e diagnósticos essenciais. O oráculo de snapshot lê modelos normalizados, não CSV; não substitui teste do adapter. Esperado não pode ser produzido pela mesma função em teste. O decoder de referência não é um validador de arquivos hostis para uso em produção.

Go pode ler `expected.json` diretamente com encoding/json nos testes. Tags especiais de floats ficam como strings no JSON de referência; isso não muda o tipo do valor no Graph. Não comparar JSON de dados com NaN sem tratamento definido.

Metamórficos: transposição troca forward/reverse; renomear IDs conserva topologia; contribuição coerente duplicada não duplica entidade; permutar inputs não escolhe vencedor de conflito; índice não altera resultado; bytes e mmap concordam. Mover uma quebra lexical pode mudar o prefixo recuperável e não exige invariância impossível.

Para alcance de grafos pequenos, usar matriz/Floyd–Warshall independente da BFS de produto. Para topologias grandes, invariantes de contagem e casos calculáveis evitam usar algoritmo cúbico como benchmark do motor.

## Recursos e falhas

Injetar erros em catálogos, readers, DiagnosticSink e Transaction. Simular falha pós-rename no adapter correto. Usar fragmentação e short writes reais. Não criar allocator público para fingir OOM recuperável em Go; usar limites explícitos e headers artificiais com overflow.

Teste de unmap/Close em paralelo com leitor não é carga suportada; não exercitá-lo de forma que possa derrubar a suite. Testar o contrato do dono e ausência de leitura depois de Close. Truncamento externo pode causar SIGBUS: risco operacional documentado, não um panic que a engine promete capturar.

Mutar snapshots de forma estruturada e saber qual invariante cada mutação viola. Uma alteração que vira outro Graph válido não precisa ser detectada sem autenticação. Fuzz deve impor tamanho razoável a inputs gerados para testar parser, não esgotar a máquina por design.

## Relato

Comando, resultado e escopo realmente exercitado. Falta de Graphviz só deixa verificação externa de DOT não executada; testes semânticos internos continuam obrigatórios. Falta de suporte de race exige registrar bloqueio desse gate, não torná-lo verde. Não acessar Neptune, buckets ou dados reais sem autorização.

## Ingestão externa

`go test ./ingest -run TestExternal -count=1` compara snapshots byte a byte,
relatórios sem tempos e diagnósticos completos entre os backends. Inclui as 12
fixtures com oráculos independentes, grupos atravessando runs, registro maior
que a arena, permutações e falhas de scratch/cancelamento/diagnóstico.

A regressão normal gera 7.200 e 72.000 registros em subprocessos, com teto fixo
para heap e verificação de queries após remoção dos nomes temporários. A
qualificação maior gera 1,2M/2,4M registros e 240 mil valores set no mesmo ID:

```sh
# TMPDIR deve ser um diretório existente em disco, com espaço para scratch.
GOPHERGRAPH_SCALE=1 go test ./ingest \
  -run 'TestExternal(GeneratedScale|SingleEntityScale)$' -count=1 -v
```

A validação CSR usa membership + ordenação estrita + contagem para provar
unicidade/cobertura, sem bitset proporcional às edges. Casos de duplicação no
mesmo dono e entre donos, em ambas as direções, protegem essa mudança.
