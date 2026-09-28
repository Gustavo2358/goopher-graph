# Exemplo de consulta Go

[shared_targets.go.txt](shared_targets.go.txt) é uma função consumidora da API planejada. Seu resultado é a interseção dos alcances reflexivos de duas origens.

Em B09, copiar a função para um exemplo/consumidor real, adicionar um main pequeno que abre snapshot e imprime IDs, compilar e executar. O main não pertence à engine e a função não recebe path, CLI ou objeto HTTP.

O arquivo é textual porque a engine ainda não existe. Sua sintaxe pode ser verificada agora; a integração real é gate futuro. Não criar stubs do core para marcar esse gate como pronto.
