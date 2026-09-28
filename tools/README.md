# Ferramentas de conferência

`python3 tools/check_package.py` confere links, DAG, layout, modelos, consultas esperadas, snapshots de referência e sintaxe dos snippets Go. Se Go estiver disponível, executa também o probe da stdlib. Python é opcional no runtime/build do produto; aqui serve como oráculo independente e usa somente biblioteca padrão.

`python3 tools/reference_snapshot.py encode fixtures/01_topology/expected.json /tmp/reference.snapshot` gera referência a partir de modelo já consolidado. `inspect` faz a operação inversa em arquivos pequenos canônicos. Não é loader CSV nem validador de segurança de produção.

`go run tools/go_semantics_probe.go` verifica premissas concretas de encoding/csv, strconv e campos LE. Não prova que o futuro adapter trata essas premissas corretamente. O probe não requer dependências externas.

Os goldens não são regenerados automaticamente pelo checker. Uma mudança intencional de contrato deve atualizar documento/layout/esperado juntos, com justificativa; não usar regeneração para esconder erro do writer.
