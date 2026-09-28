# Contratos e referências

- [API Go](api/README.md): assinaturas documentais; sem implementação.
- [Layout](snapshot_layout.json): constantes, campos, strides e seções do arquivo inaugural.
- [DAG](backlog_dag.json): dependências do backlog, sem estado de progresso.
- [Fixtures](../fixtures/README.md): inputs e modelos esperados.

`format_version=1` é um identificador interno do arquivo. Não há versões históricas de produto, migração, pins ou SHAs a gerir.

O oráculo Python produz/inspeciona modelos pequenos. Não lê CSV, não é biblioteca de produto e não substitui parser/validação Go. Testes normais da engine devem funcionar com tooling Go; Python é uma conferência independente opcional de referências.
