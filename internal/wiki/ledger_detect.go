package wiki

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Regra de reconhecimento do ledger de eventos (docs/complementary-contracts.md §4). Usada pelo
// reindex e pelo watcher: os dois decidem se um `.md` é ledger pelo mesmo código.

// ledgerPrefixBytes é o prefixo lido do arquivo para decidir. Um ledger pode ter dezenas de GB, e
// o reconhecimento não lê tudo.
const ledgerPrefixBytes = 8 << 10

// ledgerFrontmatterLines é o limite de linhas em que o frontmatter precisa fechar. A linha de
// abertura conta como a primeira.
const ledgerFrontmatterLines = 64

// ledgerEntryPrefix é o início de uma entrada: `## [timestamp] evento | título`.
const ledgerEntryPrefix = "## ["

// ledgerName diz se o nome do arquivo tem a forma do ledger: `log.md` ou `log-AAAA-MM.md`, exatos.
// Só a forma é verificada: o mês não é validado quanto ao intervalo. Não distingue caixa.
func ledgerName(name string) bool {
	if name == logFileName {
		return true
	}
	// log- (4) + AAAA-MM (7) + .md (3).
	if len(name) != 14 || !strings.HasPrefix(name, "log-") || !strings.HasSuffix(name, ".md") {
		return false
	}
	mes := name[4:11]
	for i := 0; i < len(mes); i++ {
		c := mes[i]
		if i == 4 {
			if c != '-' {
				return false
			}
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isLedgerFile diz se o arquivo em path é ledger: nome da forma `log.md` ou `log-AAAA-MM.md` e
// conteúdo que abre com entrada. Nome que não casa devolve false sem ler o arquivo. Lê no máximo
// ledgerPrefixBytes. Não segue links: quem chama já filtra links e pontos de reparse.
func isLedgerFile(path string) (bool, error) {
	if !ledgerName(filepath.Base(path)) {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, ledgerPrefixBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, err
	}
	// Leu menos que o limite: o prefixo é o arquivo inteiro.
	complete := n < ledgerPrefixBytes
	return ledgerContent(buf[:n], complete), nil
}

// ledgerLine é uma linha do prefixo. terminated diz se havia `\n` depois dela.
type ledgerLine struct {
	text       []byte
	terminated bool
}

// nextLine separa a primeira linha de b e devolve o resto. ok é false quando b está vazio.
func nextLine(b []byte) (line ledgerLine, rest []byte, ok bool) {
	if len(b) == 0 {
		return ledgerLine{}, nil, false
	}
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return ledgerLine{text: trimCR(b)}, nil, true
	}
	return ledgerLine{text: trimCR(b[:i]), terminated: true}, b[i+1:], true
}

// trimCR tira o `\r` final de uma linha (CRLF).
func trimCR(b []byte) []byte {
	return bytes.TrimSuffix(b, []byte("\r"))
}

// ledgerContent aplica a regra de conteúdo ao prefixo. complete diz se o prefixo é o arquivo
// inteiro. Regras:
//   - se a primeira linha é `---`, há frontmatter: ele precisa fechar com `---` até a linha 64
//     (inclusive). Sem fechamento nesse limite, não é ledger;
//   - a linha seguinte ao frontmatter, ou a primeira linha sem frontmatter, precisa ser uma
//     entrada `## [...`. Linha vazia antes da entrada decide contra;
//   - a linha de decisão precisa estar completa dentro do prefixo (terminada por `\n`, ou o arquivo
//     inteiro). Se o limite de 8 KiB corta antes, não é ledger;
//   - arquivo vazio não é ledger.
//
// Na dúvida, a resposta é false.
func ledgerContent(prefix []byte, complete bool) bool {
	b := bytes.TrimPrefix(prefix, []byte(utf8BOM))
	first, rest, ok := nextLine(b)
	if !ok {
		return false
	}

	entry := first
	if string(first.text) == fenceLine {
		if !first.terminated && !complete {
			return false
		}
		n := 1
		closed := false
		for !closed {
			if n >= ledgerFrontmatterLines {
				return false
			}
			var line ledgerLine
			line, rest, ok = nextLine(rest)
			if !ok {
				return false
			}
			n++
			closed = string(line.text) == fenceLine
		}
		entry, _, ok = nextLine(rest)
		if !ok {
			return false
		}
	}

	if !entry.terminated && !complete {
		return false
	}
	if !bytes.HasPrefix(entry.text, []byte(ledgerEntryPrefix)) {
		return false
	}
	return bytes.IndexByte(entry.text[len(ledgerEntryPrefix):], ']') >= 0
}
