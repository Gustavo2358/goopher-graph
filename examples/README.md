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

`filtered` mantém a filtragem de propriedades restrita aos vizinhos:
`Out().HasLabel(label).Has(key, value)`, com os quatro argumentos existentes.
Isso evita examinar propriedades de outros nodes quando o label é comum.
Para a fixture `wasmquery/testdata/selection`, passe `S L tag X`.

`wasm/select_labels` exercita a seleção composta global genérica: recebe
propriedade, valor string e zero ou mais labels (`tag X L M`), usando opcode 27.
Sem labels, seleciona vazio. Esse exemplo é compilado para a CLI, separado do
registry padrão do servidor. A indexação opcional de `tag` usa a flag genérica
`--index-property`. [Contrato e comandos](../docs/WASM.md).
