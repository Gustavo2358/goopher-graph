# Iniciar a sessão

Use esta pasta como raiz. Não é necessário carregar toda a documentação em um único prompt nem consultar pacotes anteriores.

## Prompt

> Implemente o GopherGraph especificado nesta pasta. Leia `AGENTS.md`, `SCOPE.md`, `PROGRESS.md` e `BACKLOG.md`; consulte os contratos da próxima fatia liberada. Implemente incrementalmente, execute os testes e avance após cada gate verde, sem pedir confirmação a cada etapa. Preserve o escopo e escolha livremente os detalhes internos. Ao encerrar, atualize `PROGRESS.md` e informe o que funciona, os testes realmente executados e o próximo passo.

## Primeira sessão

Começar em **B00**. Criar o módulo e o primeiro teste real; não preencher o repositório com stubs para fingir que o backlog está pronto. Os arquivos `.go.txt` em `spec/api/` são assinaturas de referência, não fontes de produto para compilar diretamente.

## Retomar

O mesmo prompt serve em outra sessão. O estado está somente em `PROGRESS.md`. As dependências de cada cartão determinam o próximo trabalho; não existe checkpoint por hash de commit, release ou aprovação formal.

Leia a arquitetura uma vez; nas demais etapas, recupere apenas os contratos necessários. Uma lacuna privada de implementação é trabalho do agente. Uma contradição real de requisito é tratada localmente, com explicação e teste, sem iniciar outra campanha documental.
