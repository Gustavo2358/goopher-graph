# CLI local

Um binário `gophergraph`, com comandos fixos. Essa seleção simples não é um executor de queries nem uma DSL. Consultas adicionais podem ser outros programas Go sem serem incorporadas à CLI.

## Build

```sh
gophergraph build \
  --nodes ./input/nodes \
  --edges ./input/edges \
  --output ./graph.snapshot \
  --index-property sigla
```

`--nodes`, `--edges` e `--output` são obrigatórios. Nodes/edges são diretórios distintos e existentes, mesmo quando vazios. Enumerar só entries imediatas, sem seguir symlinks e sem filtro por extensão. O usuário é responsável pelo conteúdo; a engine rejeita individualmente o inválido por header. Não adivinhar papéis.

`--index-property KEY` pode repetir; deduplicar/ordenar opções. Sem essa opção, apenas índices obrigatórios de labels/IDs. `--max-record-bytes N` tem default 67108864, mínimo 1024; aplica-se inclusive ao header. Não é um limite global de memória nem desculpa para truncar o grafo.

Output não pode coincidir nem ficar dentro de nodes/edges; validar caminhos resolvidos e aliases locais razoáveis no adapter. Rejeitar catálogos idênticos. O produto não é um sandbox contra filesystem adversarial e não cria rede/remote.

Ao fim, stdout recebe um resumo legível: publicação, completeness, fontes, registros vistos/rejeitados/staged, conflitos e contagens finais. Diagnósticos vão a stderr. Carga parcial publicada com sucesso retorna 0, com `PARTIAL` explícito; não anunciar completa. O bit de parcialidade fica no snapshot. Empty válido também publica; EMPTY_GRAPH é aviso, não aborto.

### Recursos de build

- `--memory-budget`: bytes para buffers de ordenação externa; padrão 67108864,
  mínimo 1048576. Não é um limite global de RSS ou do heap do processo.
- `--temp-dir`: diretório pai existente para scratch privado; padrão é o diretório
  de `--output`. Precisa ficar fora dos catálogos, inclusive quando há symlinks.
- `--max-record-bytes` continua limitando cada registro/header, independentemente
  do orçamento de ordenação. Um token maior que a arena vira um run separado.

Colunas, strings e índices são construídos em disco. Escolha armazenamento com
espaço livre para os passes e evite tmpfs em cargas maiores que a RAM. Falhas de
scratch são operacionais; publicação anterior permanece intacta. SIGKILL ou queda
do processo podem deixar diretórios `.gophergraph-build-*`; remova somente os de
processos encerrados. Ver [garantias e medições](BOUNDED_INGEST.md).

## Consultas

```sh
gophergraph territory --snapshot ./graph.snapshot --node PROGRAM:A
gophergraph anti-territory --snapshot ./graph.snapshot --node PROGRAM:B
gophergraph between --snapshot ./graph.snapshot --from PROGRAM:A --to PROGRAM:B

gophergraph territory --snapshot ./graph.snapshot --node PROGRAM:A \
  --edge-label CALLS --edge-label READS --format dot --output ./territory.dot
```

`--snapshot` e identificadores de origem/fim são obrigatórios. Verificar presença da flag, **não `value != ""`**: um ID externo vazio é válido e pode ser passado como `--node=""`. Nomes não são normalizados. IDs inexistentes geram diagnóstico e exit 3.

Sem `--edge-label`, todas as relações. Com uma ou mais, união dos labels solicitados. Se nenhum existir, filtro vazio e somente alcance reflexivo; não usar nil por engano. `--edge-label=""` é erro de uso porque label vazio não é válido.

`--format ids` é padrão. Para território/antiterritório, listagem omite origem; `--include-origin` a inclui. Between lista todos os membros. DOT, JSON e GGPB incluem todos os membros do subgrafo. `--output` omitido escreve em stdout; quando presente escreve no arquivo local. Biblioteca só recebe io.Writer.

## JSON

As três consultas aceitam `--format json`, com o [contrato de subgrafo completo](JSON.md).
`--include-origin` continua restrito a IDs. JSON contém parâmetros, contagens e
`partialSnapshot`; o aviso de carga parcial permanece em stderr.

## CSV de IDs

Cabeçalho `id`, seguido de uma coluna de IDs externos em ordem canônica. Não usar uma linha textual bruta por ID: IDs podem conter vírgulas, aspas e newline.

Usar `encoding/csv.Writer` com complemento para campo vazio: uma linha com ID vazio deve ser **`""`**, não uma linha em branco, que leitores CSV poderiam ignorar. Não misturar diagnóstico/progresso com dados de consulta em stdout. Verificar erro de Flush/Close. Saída de consulta não é um CSV Neptune de reingestão.

Snapshot com carga parcial emite aviso em stderr antes de consultar. A API informa metadata para outros drivers. As respostas cobrem todo o snapshot aceito, não os registros rejeitados anteriormente.

## Exit codes

| Código | Significado |
|---:|---|
| 0 | Operação concluída; build pode ser COMPLETE ou PARTIAL |
| 1 | Falha operacional, cancelamento ou publicação não confirmada |
| 2 | Uso/flags/caminhos de configuração inválidos |
| 3 | Node/edge requerido não encontrado |

`PublishedUncertain` sai 1 e informa que o nome novo já está visível; não alegar rollback. Erro escrevendo relatório depois de publicar também sai 1 com publicação preservada. `--help` sai 0 sem abrir dados. SIGINT cancela contexto e limpa staging quando ainda não publicado.

Somente main chama os.Exit, depois de a função run retornar e seus defers encerrarem recursos. Não usar log.Fatal no meio do fluxo. Uma CLI pequena pode usar `flag.FlagSet`; framework externo não é necessário.

## GGPB e decode

`territory`, `anti-territory` e `between` aceitam `--format ggpb` para resultados
Protobuf autocontidos, versionados e limitados por batch. [Contrato GGPB](GGPB.md).

```sh
gophergraph territory --snapshot graph.snapshot --node A --format ggpb --output result.ggpb
gophergraph decode --input result.ggpb --format json --output result.json
```

Decode não recebe snapshot. Erros de framing, versão, tipos, contagens, End e I/O
retornam código 1; flags inválidas retornam 2. Destinos que identificam o mesmo
arquivo de entrada são rejeitados antes de criar/truncar o output.
