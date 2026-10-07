# API Go e contratos de uso

[spec/api](../spec/api/README.md) contém assinaturas de referência. Elas fixam comportamento e direção das dependências, não campos privados. O agente pode organizar helpers internos sem transformar a API em um framework.

## Graph

`graph.Graph` é concreto, com campos privados. IDs públicos distintos: NodeID, EdgeID e StringID. O objeto aberto é imutável; contagens e flag de carga parcial são metadados copiados. Zero value não representa um Graph aberto.

`FindNode`/`FindEdge` recebem ID externo string e retornam ID interno ou ErrNotFound. String vazia é ID válido e não equivale a argumento ausente. Getters validam ranges e retornam erro tipado. Não panic por input externo. Métodos de contagens/metadados copiados não precisam acessar o mapping; acesso a dados após Close retorna ErrClosed.

Métodos de adjacência retornam iterador por valor. `Next` não aloca por edge; `Edge` expõe ID, source, target, label e neighbor. Forward: neighbor=target; reverse: neighbor=source. Source/target sempre conservam orientação lógica. Ler Value sem Next bem sucedido é uso incorreto; `Err` informa falha de iteração.

Labels são StringIDs do mesmo dicionário; nomes são resolvidos uma vez fora do hot loop. Métodos de strings retornam cópias Go seguras, não aliases inseguros do mapping. APIs de labels/propriedades podem usar iteradores ou cópias pequenas, mas não expõem slices graváveis do armazenamento.

## Valores e filtros

Value tem tag e payload canônico encapsulados, com construtores/acessores tipados. Não usar `map[string]any` como modelo permanente. `Equal` implementa o contrato de tags, NaN e zeros assinados; datas continuam texto, sem equivalência de instantes.

Propriedade tem key e Value. Todas as propriedades, inclusive set de node e escalares de edge, são acessíveis. Busca por label/propriedade retorna conjunto vazio quando chave/label não existe. Índice presente e scan devem retornar exatamente o mesmo conjunto.

`NodeSet` e `EdgeSet` são tipos separados com identidade de Graph. Interseção/união rejeitam outro Graph, mesmo que tenha tamanho igual. Bitmaps mutáveis são privados; o chamador pode montar um NodeSet, mas Subgraph copia o conjunto recebido para não ser invalidado por mutação posterior do input. Retorno de metadados não concede acesso ao backing.

## Consultas

`query.Reachable(ctx,g,start,direction,options)` retorna NodeSet reflexivo. `query.FromNodes` seleciona todas as edges permitidas cujos endpoints pertencem ao conjunto e retorna Subgraph. `Territory`, `AntiTerritory` e `Between` são funções convenientes que compõem essas primitivas.

`Options.EdgeLabels == nil` significa todas as relações; slice não-nil vazio significa nenhuma. IDs de label são resolvidos no mesmo Graph e comparados como dados. Labels inexistentes escolhidos na CLI produzem filtro vazio, não fallback para todas as edges.

Subgraph tem nodes e edges únicos e vínculo ao Graph. A API permite iterar os membros sem copiar listas enormes e adicionar elementos por construção controlada, sem dangling edges. Resultados usam heap Go, não precisam de Free. O Graph deve continuar aberto enquanto o consumidor usar seus metadados; resultados não autorizam Close concorrente.

Cancelar consulta retorna erro e nenhum resultado anunciado como completo. Não há cutoff oculto, amostragem ou enumeração de caminhos. Snapshot parcial não transforma a query em incompleta sobre os dados armazenados, mas a origem parcial é exposta em metadata.

## Ingestão e persistência

`ingest.Build` recebe contexto, dois Catalogs, Decoder, DiagnosticSink e Options. Retorna Graph consolidado, Report e error. Report parcial + error nil é sucesso operacional com rejeições. Graph ainda não está publicado: snapshot.Write é operação separada, com Publication explícita. `Report.Times` informa tempos de leitura/merge e canonicalização/CSR/índices para medição local; esses tempos não são persistidos.

`Options.NodePropertyConflictPolicy` configura valores diferentes de
propriedades single de nodes. `Options{}` usa `LastWins`; `FirstWins` e
`DropConflictingProperty` são explícitos. Arquivos seguem a ordem de bytes
da chave do catálogo; registros seguem a ordem de emissão do decoder.
Propriedades ausentes e registros rejeitados não participam. Sets, conflitos
de cardinalidade e edges mantêm suas regras. Valor de enum inválido é erro
de configuração, antes de abrir catálogos ou scratch. Resoluções geram warning
com política e origem vencedora, incrementam
`Report.ResolvedNodePropertyConflictGroups` e não tornam a carga parcial por
si só. [Semântica e compatibilidade](INGEST.md).

`snapshot.Open(ctx,Source)` abre/valida o backing e transfere lifecycle para Graph. `snapshot.Write(ctx,g,Sink)` serializa, valida staging e publica. Não receber um path no domínio para facilitar a CLI; path fica em Source/Sink concretos.

Para ingestão limitada, configure `Options{Scratch: filesystem.Scratch{Dir: dir},
MemoryBudget: 16 << 20}`. `MemoryBudget=0` com Scratch seleciona 64 MiB; valores
positivos exigem Scratch e devem ser pelo menos 1 MiB. `Options{}` mantém o
backend heap existente. A CLI sempre fornece Scratch. O Graph retornado pelo
caminho externo mantém mappings das colunas; chame Close mesmo sem publicar.
O builder fecha o workspace em sucesso/erro; mappings sobrevivem à remoção dos
nomes temporários até Graph.Close. O chamador de biblioteca deve manter scratch
fora dos catálogos. [Limites de memória](BOUNDED_INGEST.md).

A CLI compõe Build + Write. Um driver futuro pode chamar exatamente as mesmas funções, sem importar `cmd/` nem repetir regras de dados.

## Recursos e concorrência

Close sequencial é idempotente. O Graph guarda release de backing; o chamador garante que consultas/iteradores terminaram antes de fechar. Não há refcount automático, finalizer ou lock por edge. Testar leituras concorrentes com workspaces separados usando race detector, incluindo erros e cancelamento.

Evitar retorno de aliases mutáveis em metadados/sets. Não guardar logger global, graph singleton, estado de locale ou contexto global. Nenhum método de biblioteca chama os.Exit/log.Fatal para responder a input.

## Extensão real

[shared_targets.go.txt](../examples/shared_targets.go.txt) demonstra uma query composta por duas buscas e uma interseção. Em B09, implementá-la como programa em módulo consumidor temporário ou exemplo separado, usando apenas packages públicos. Compilar e executar sem modificar graph, snapshot ou adicionar um registry.

Os arquivos `.go.txt` são documentação de assinaturas: checagem sintática do pacote não prova resolução de imports, tipagem da engine ou implementação das funções. Esses gates só ficam verdes sobre o produto real.

## Defaults de opções

`Options{}` usa limites padrão: MaxRecordBytes=67108864 e MaxColumns=65536. Zero nesses campos significa default na API; valor explícito fora da faixa é erro de configuração. Para MaxColumns, a faixa configurável é 1..65536. A CLI distingue ausência de flag de `--max-record-bytes=0`, que é uso inválido. Contexto nil não é suportado: o driver usa Background/sinal e propaga cancelamento.

FindString informa `(id, found, error)` para distinguir ausência de string de Graph fechado. Não transformar ErrClosed em conjunto vazio. As operações de conjunto não leem bytes do mapping, mas sua identidade continua sendo a do Graph em que nasceram.

## GGPB

`ggpb.Write`, `ggpb.Emit`, `ggpb.EmitEncoded`, `ggpb.NewReader`/`Next` e `ggpb.JSON` formam a API de
exportação/consumo incremental. O contrato lógico é `ggpb/pb/result.proto`.
Ver [GGPB](GGPB.md) para ownership, limites, framing e compatibilidade.
