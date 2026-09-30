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
