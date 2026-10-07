# Consulta Go própria

[shared_targets/query.go](shared_targets/query.go) calcula a interseção dos alcances reflexivos de duas origens, usando somente `graph` e `query`. O Graph pertence ao chamador.

Após criar o snapshot da fixture conforme o README principal:

```sh
go run ./examples/shared_targets/cmd bin/graph.snapshot A B
```

A saída contém `"B"`, `"D"` e `"F"`, um ID por linha JSON. O [programa consumidor](shared_targets/cmd/main.go) abre o mmap, chama a função e fecha explicitamente o Graph. Não altera a engine nem registra plugins.

O E2E compila e executa um consumidor em módulo temporário separado, sem rede. [shared_targets.go.txt](shared_targets.go.txt) preserva a assinatura de referência original.


## WASM

`wasm/shared_targets`, `wasm/between` e `wasm/filtered` são programas externos à
engine, escritos com `wasmquery/sdk`. Compilação com Go `wasip1/wasm`, argumentos
e semântica estão em [docs/WASM.md](../docs/WASM.md).

`filtered` usa seleção composta genérica e interseção com os vizinhos, mantendo
os quatro argumentos. O asset instalado versão 2 deve acompanhar um host com
opcode 27. Para testar com dados fictícios, construa a fixture
`wasmquery/testdata/selection` e passe `S L tag X`; a indexação opcional de `tag`
usará a flag genérica `--index-property`. [Contrato e comandos](../docs/WASM.md).
