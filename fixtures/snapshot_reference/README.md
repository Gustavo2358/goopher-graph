# Snapshots de referência

Seis arquivos pequenos produzidos pelo oráculo a partir de modelos normalizados, com magic ASCII `GOPHGRPH` e o layout inaugural. [cases.json](cases.json) associa cada um ao modelo e tamanho esperado.

Não foram produzidos por uma engine Go implementada. Testar reader do produto contra estes bytes e writer do produto contra estas referências; round-trip próprio sozinho não basta.

A conferência compara bytes diretamente e não exige SHA, pin ou histórico de versões. O gerador não roda automaticamente para consertar um teste falho.
