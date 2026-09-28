# Protocolo de fechamento lean

## Fechar uma fatia

Comportamento entregue, testes focais e regressão disponíveis executados, nenhum stub travestido de funcionalidade, contratos ainda coerentes. Atualizar uma linha e uma nota curta em PROGRESS. Não criar relatório, certificado ou ADR para cada função.

Exemplo de nota útil:

> B04 concluída: nodes precedem edges; registros inválidos e fontes ruins são isolados. Executei os testes de ingestão e a regressão; ambos passaram. Próxima: B05, adapters de filesystem.

Substituir por comandos/resultados reais; o exemplo não é evidência de teste deste pacote.

## Encerrar sessão

Registrar fatia atual, próximo passo e bloqueio concreto quando houver. Relatar ao usuário somente comportamento implementado, testes executados e próximo passo. Não repetir o plano inteiro nem anexar logs enormes sem necessidade. Após um gate verde, avançar autonomamente enquanto houver contexto útil.

## Fechar o produto em B12

1. Reproduzir build/teste a partir do repositório local sem artifacts de produto pré-existentes. Usar as dependências disponíveis pelo meio autorizado; não exigir rede só para a prova.
2. Rodar testes sem cache, vet, conferência de gofmt, build CGO_ENABLED=0, race em ambiente compatível e campanhas de fuzz especificadas.
3. Executar carga completa e parcial reais, três consultas, escaping, índice/scan, heap/mmap, publicação sob falha e consumidor de query separado.
4. Confirmar SCOPE, atualizar README com comandos efetivos e manter PROGRESS como estado final. Anotar medições de B11 e limitações reais, sem SLA inventado.

Pendências de requisitos obrigatórios impedem marcar B12 concluído. Ideias fora do escopo (HTTP, S3, caches, novas consultas) não impedem o fechamento.

## Não é necessário

Pins de patch, SHAs de commits, hashes de aprovação, múltiplos arquivos de status, release number, screenshots de cada teste, burocracia de review ou autorização do usuário para cada gate. go.mod/go.sum e commits locais normais continuam existindo; não são a cerimônia que estamos evitando.

## Limites do encerramento

Compatibilidade Neptune é qualificada pelos contratos/fixtures, não por uma carga em cluster que nunca foi executada. Sem corpus real, não inventar taxa de sucesso corporativa. Os goldens documentais não comprovam a engine; a engine só está pronta após seus próprios gates.
