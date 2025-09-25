# Transferência de Arquivos Confiável sobre UDP (Go)

Lucas Yukio Fukuda Matsumoto - Matrícula 2516977

Aplicação cliente-servidor em Go que implementa transferência confiável sobre UDP: segmentação (1 KiB), cabeçalho binário customizado, CRC32 por segmento, ordenação por número de sequência, recuperação com NACK/retransmissão e verificação final SHA-256 do arquivo completo. Inclui GUI (Fyne) e binários CLI (sem GUI).

Principais recursos:
- Protocolo de aplicação próprio (controle em JSON: REQ, META, ERR, EOF, NACK; dados binários com cabeçalho UD v1).
- Simulação de perda no cliente: taxa aleatória (drop-rate) single-shot (cada sequência pode ser descartada no máximo uma vez). Seed gerado automaticamente a cada execução.
- GUI em Fyne (com fallback de renderização por software no Windows) e CLI headless para demonstração.

## Protocolo do datagrama UDP

- Controle (JSON, UTF-8) com campo `type`:
  - `REQ` cliente→servidor `{type:"REQ", version:1, path:"caminho/arquivo"}`
  - `META` servidor→cliente: `{type:"META", filename, total, size, sha256, chunk}`
  - `EOF` servidor→cliente: fim do envio inicial
  - `NACK` cliente→servidor: `{type:"NACK", missing:[...]}`
  - `ERR` servidor→cliente: `{type:"ERR", message:"..."}`
  - `LIST` cliente→servidor: `{type:"LIST"}`
  - `LST` servidor→cliente: `{type:"LST", files:[...]}`
- Dados (binário, big-endian): magic `UD`, version `1`, flags `0`, seq(u32), total(u32), size(u16), crc32(u32) + payload (<= 1024 bytes)
- SHA-256 para o arquivo completo enviado em META; cliente compara ao final.
- Segmentação com cabeçalho customizado e CRC32 por segmento; Fixado ChunkSize = 1024 bytes (evita fragmentação IP típica para MTU ~1500).
- NACK com lista de sequências faltantes para retransmissão específica.
- Timeout customizável

## Build (Windows Powershell)

```powershell
# GUI: gera .exe em bin/
pwsh .\scripts\build-gui.ps1  # escreve em bin/ server-windows-amd64.exe e client-windows-amd64.exe

# CLI: gera .exe em bin/
pwsh .\scripts\build-cli.ps1
```

Use o `build.ps1` para compilação multiplataforma.

## Execução – GUI (Fyne)

Servidor GUI:
```powershell
.\server.exe
# Na janela: Host 127.0.0.1, Porta 19000, Diretório base, clique Iniciar
```
Cliente GUI:
```powershell
.\client.exe
# Host 127.0.0.1, Porta 19000
# Clique "Listar arquivos no servidor" para popular a lista.
# Selecione um arquivo na lista OU digite manualmente um nome (para testar arquivo inexistente).
# Saída pode ficar em branco: salvará automaticamente como recv_<arquivo>
# (opcional) Drop rate ex.: 0.05  (seed é gerado automaticamente)
# Timeout ex.: 2s   | Retries ex.: 5
# Clique Iniciar e acompanhe gráfico e logs
```

GUI – Gráficos e Logs:
- Taxa recente (B/s): gráfico de linha com a taxa instantânea calculada a cada ~200 ms.
- Segmentos/s: gráfico de linha com o número de segmentos válidos recebidos por segundo.
- Bytes restantes (KB): gráfico de linha decrescente com (tamanho_total - bytes_recebidos)/1024.
- Logs: área com tags de severidade [INFO], [ERR], [DROP], [META], [EOF].

## Execução – CLI (sem GUI)

Servidor CLI:
```powershell
.\bin\cli-server.exe --host 127.0.0.1 --port 19000
```
Cliente CLI:
```powershell
# Uso básico (sem simulação de perda)
.\bin\cli-client.exe -t "127.0.0.1:19000/test.bin" --timeout 2s --retries 5 -o recv_test.bin

# Com simulação de perda aleatória single-shot (cada seq só pode ser perdida 1 vez)
.\bin\cli-client.exe -t "127.0.0.1:19000/test.bin" --drop-rate 0.05 --timeout 2s --retries 5 -o recv_test.bin
# Também funciona com @ no início: -t "@127.0.0.1:19000/test.bin"
```
Saída mostra META, progresso, rounds de NACK e integridade final (SHA-256).

## Escolha de arquivo (qualquer tipo)

- Deve-se escolher qualquer arquivo existente no servidor para enviar.
- Por padrão, o servidor usa o diretório base definido na GUI (campo “Diretório base”) ou “.” (pasta atual).
- No cliente, informe o nome do arquivo (relativo ao diretório base do servidor), por exemplo:
  - Arquivo: foto.jpg
  - Arquivo: docs/relatorio.pdf

## Observações de projeto
- ChunkSize = 1024 (1 KiB): margem para MTU Ethernet (~1500) e cabeçalhos IP+UDP (~28) + cabeçalho de aplicação.
- Ordenação: número de sequência no cabeçalho dos dados.
- Detecção de perda: lacunas em `seq` e ociosidade levam a rounds de `NACK`.
- Integridade: CRC32 por segmento; SHA-256 final do arquivo.
- Fluxo/Janela: envio simples (blast) com retransmissões sob demanda; pode ser estendido para janela deslizante e ACKs cumulativos.
- Política de perda (cliente): drop-rate aplicado apenas na primeira vez que ele chega; retransmissões nunca são descartadas novamente, permitindo recuperação determinística.
