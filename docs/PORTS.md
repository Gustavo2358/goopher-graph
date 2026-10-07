# Ports locais, composição e recursos

## Princípio

Interfaces Go pequenas descrevem comportamento externo. `io.Reader`, `io.ReadCloser` e `io.Writer` são usados diretamente onde bastam; não os duplicar com interfaces renomeadas. Catálogo, decoder de registros, diagnóstico e publicação precisam de contratos adicionais próprios.

Operações são síncronas. Não há container de DI, registry, barramento global ou goroutine por interface. O chamador compõe implementações concretas.

## Ingestão

As declarações estão em [spec/api/ingest_ports.go.txt](../spec/api/ingest_ports.go.txt). Os tipos vivem no package `ingest/ports`, sem importar ingest nem adapters.

| Port | Papel |
|---|---|
| Catalog | List(ctx) enumera entries; Open(ctx,key) retorna io.ReadCloser |
| Decoder | New(ctx,role,entry,reader,limits) valida header e cria RecordReader |
| RecordReader | Next(ctx) retorna record normalizado, rejeição delimitada ou erro terminal/EOF |
| DiagnosticSink | Emit(ctx,Diagnostic) registra evento; retorna erro se não puder registrá-lo |
| Scratch | New(ctx) cria um Workspace privado por build |

As interfaces de scratch estão em [ingest/ports/scratch.go](../ingest/ports/scratch.go).
Workspace cria arquivos com Writer/ReaderAt/Closer, remove arquivos, mapeia
arquivos completos e fecha todos os arquivos remanescentes. Map retorna um
Mapping com Bytes/Close: a view permanece estável mesmo após Remove/Close do
workspace. O dono deve fechar o Mapping separadamente. Build transfere apenas
os mappings finais ao Graph; todos os outros recursos são liberados antes do
retorno. Erros de scratch nunca são classificados como rejeições de fonte CSV.

Catalog fornece coleção finita, com keys únicas e descriptors estáveis durante a carga. Entry distingue regular de não regular. A aplicação ordena entries por key e rejeita não regulares com diagnóstico. List falhando no catálogo raiz é fatal; Open/read de uma entry é recuperável.

O adapter filesystem usa somente o diretório declarado, sem recursão nem symlink, e revalida o tipo ao abrir. Não seleciona por extensão. O futuro adapter S3 usa keys/prefixes sem ensinar esses conceitos ao builder.

O stream pertence à orquestração. Decoder e RecordReader apenas o emprestam: não o fecham. O processamento de uma fonte ocorre em função local com Close garantido antes de passar para a seguinte. O reader não exige seek/reopen. Buffers/eventos podem ser reutilizados na próxima chamada; ingest copia o que retém. Nenhum destructor artificial para simples objetos heap Go.

Record é genérico: role, ID, endpoints quando edge, labels e propriedades (key, valor tipado, cardinalidade). Origem de diagnóstico é opaca. Nomes reservados `~id`/`~from`, estado de quotes e headers não entram no modelo de grafo.

First/last-wins usa chave do catálogo em bytes, ordem de emissão de `Next` e,
se um decoder fornecer várias contribuições da mesma chave em um evento,
ordem em `Record.Properties`. `Location` serve apenas ao diagnóstico; sua
numeração não define precedência. O builder atribui a sequência antes do sort.

Event contém exatamente um Record ou uma Rejection; warnings podem acompanhar Record válido. `io.EOF` encerra fonte normalmente. Um registro inválido não vem como erro fatal genérico. Erros terminais do reader são classificados (source I/O, formato irrecuperável ou recurso/cancelamento) pela orquestração.

Emitir rejeições uma vez no orquestrador. Warnings só acompanham contribuições validadas; um record rejeitado não dispara um segundo conjunto de warnings intermediários. Falha do DiagnosticSink encerra o build antes da publicação. Não imprimir propriedades inteiras nos logs.

## Classes de erro de fonte

Header inválido: rejeitar arquivo e continuar. Erro de CSV em registro com fim confiável: evento RejectedRecord, continuar. Delimitação perdida ou I/O após prefixo: conservar registros completos anteriores, encerrar fonte e continuar catálogos. Limite por registro recuperável: rejeitar registro. Cancelamento e limite estrutural do builder: erro global. Não classificar por substring da mensagem.

## Snapshot

As declarações estão em [snapshot_ports.go.txt](../spec/api/snapshot_ports.go.txt). Backing contém bytes contíguos estáveis e Close. Seus Bytes só são visíveis no codec/integração privada, nunca como buffer gravável da API Graph.

`Source.Acquire(ctx)` transfere um Backing para `snapshot.Open`. Se Acquire falhar, o adapter limpa o próprio recurso. Se a validação falhar, Open fecha o Backing. Em sucesso, o Graph assume o release. O contexto de Source pode deixar de existir depois disso; Backing deve reter tudo de que precisa.

`Sink.Begin(ctx,totalSize)` inicia Transaction privada. O codec escreve em sequência, chama Seal para obter view estável, valida, fecha essa view e chama Commit. Transaction implementa `io.Writer`, Seal, Commit e Abort. `Write` permite short write conforme o contrato de io.Writer; codec deve tratá-lo, nunca perder bytes. Abort é idempotente e não remove destino já publicado.

| Estado | Significado |
|---|---|
| NotPublished | Destino novo não foi publicado; anterior permanece intacto |
| PublishedDurable | Rename e sincronização final concluídos |
| PublishedUncertain | Rename aconteceu, mas durabilidade final não foi confirmada |

Commit devolve estado e erro. Erro após rename não pode ser reportado como NotPublished. Cancelamento antes do commit aborta staging; depois de rename é necessário concluir/verificar sync e informar o estado real, sem prometer rollback. Falha de log/relatório depois de publicado também não despublica.

Seal não transfere ownership do staging: a view tem lease próprio e é liberada antes de Commit/Abort. Um source de memória usa owner sem recurso de SO; mmap usa Munmap. O Graph não conhece a diferença.

## Saída

`dot.Write(io.Writer, Graph, Subgraph)` recebe o writer e não abre arquivos. O driver escolhe stdout/arquivo e trata flush/Close. CSV de IDs segue o mesmo princípio; o caso de ID vazio tem regra explícita na CLI.

## Prova de desacoplamento

B04 executa ingestão com catálogos/streams de memória e falhas programadas. B07 executa codec com source/sink em memória. B05/B08 substituem por filesystem/mmap sem alterar algoritmos nem resultados. Testar fragmentação de streams, n>0 com erro, short write, Seal/Commit falho e cleanup.

Esse é o teste de ports and adapters; não é necessário implementar S3, HTTP ou inventar mais interfaces para demonstrá-lo.
