# Ferramentas de conferência

`python3 tools/check_package.py` confere links, DAG, layout, modelos, consultas esperadas, snapshots de referência e sintaxe dos snippets Go. Se Go estiver disponível, executa também o probe da stdlib. Python é opcional no runtime/build do produto; aqui serve como oráculo independente e usa somente biblioteca padrão.

`python3 tools/reference_snapshot.py encode fixtures/01_topology/expected.json /tmp/reference.snapshot` gera referência a partir de modelo já consolidado. `inspect` faz a operação inversa em arquivos pequenos canônicos. Não é loader CSV nem validador de segurança de produção.

`go run tools/go_semantics_probe.go` verifica premissas concretas de encoding/csv, strconv e campos LE. Não prova que o futuro adapter trata essas premissas corretamente. O probe não requer dependências externas.

Os goldens não são regenerados automaticamente pelo checker. Uma mudança intencional de contrato deve atualizar documento/layout/esperado juntos, com justificativa; não usar regeneração para esconder erro do writer.

## Medir ingestão

`go build -o bin/buildmeasure ./tools/buildmeasure` cria um driver de medição
isolado. `--input DIR` lê um corpus gerado por `tools/benchdata`. Sem orçamento,
mede o caminho heap; `--memory-budget 16777216` seleciona o caminho externo,
com scratch no diretório do input (ou `--temp-dir DIR`). Escreve `graph.snapshot`
nesse diretório, portanto use um corpus de medição dedicado.

A saída JSON separa build/publicação, fases do relatório, pico de HeapAlloc
amostrado a cada 10 ms, TotalAlloc, heap com o Graph vivo após GC, bytes escritos
em scratch, pico de scratch, mappings e arquivos abertos. O GC final serve apenas
para medir retenção; não altera o algoritmo ou os tempos de build/publicação.
`--profile PATH` captura perfis de heap quando o pico aumenta 128 MiB; tem custo
adicional e o perfil reflete o último ciclo de GC. RSS é medido separadamente com
GNU time. Ver [reprodução](../docs/BOUNDED_INGEST.md).

## Medir resultados e consumidor

`tools/resultmeasure` separa open/validate, query e encoding, ou mede um consumidor
incremental JSON/GGPB em outro processo. `tools/ggpb_campaign.py` alterna formatos
em cinco repetições nas fixtures determinísticas de benchdata. Tempos, CPU,
allocations, bytes e peak RSS por executable são emitidos em JSONL.
[Reprodução e resultados](../docs/GGPB_BENCHMARKS.md).

`tools/ggpb_python.py` é um consumidor opcional com o runtime oficial Protobuf,
sem acumular o resultado. Requer result_pb2.py gerado do schema, fora do build Go.
[Comandos](../docs/GGPB.md#regenerar-e-validar-interoperabilidade).

`tools/encodingmeasure` mede encoding residente sobre snapshot aberto, com warmup e perfis pprof opcionais. `--query-each` executa queries novas antes de cada encoding e reporta query/encode/CPU separadamente. `tools/ggpb_checkpoints.py` mede quatro casos em processos cold e residentes usando fixtures já construídas; não baixa dependências. Experimentos de concorrência/geometria são opt-in nos testes ggpb e não introduzem workers no produto. [Campanha de otimização](../docs/GGPB_BENCHMARKS.md).
