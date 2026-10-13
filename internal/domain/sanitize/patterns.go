package sanitize

import "regexp"

// rule é um padrão de segredo com o rótulo que aparece no marcador `[REDACTED:<rótulo>]`.
type rule struct {
	label string
	re    *regexp.Regexp
}

// rules são os 18 padrões de DV §7.2, na ordem da nota T003 (do mais específico ao mais geral).
// Ordem importa: um trecho redigido por um padrão anterior não chega aos seguintes.
//
// Todos são RE2 (`regexp` do Go): sem lookaround nem backreference. O ajuste de ADR-12 é o
// padrão `jwt`, que exige `eyJ` no primeiro segmento para não redigir identificadores como
// `internal.storage.writer`.
//
// `credential_path` aceita também `~` como prefixo (`~/.ssh`), ajuste de ADR-12.
//
// Fronteiras de palavra (`\b`): padrões com prefixo literal específico NÃO usam `\b` no início.
// Um segredo colado em letras (`aaaaghp_...`) não tem fronteira e escapava para o armazenamento,
// o que INV-06 e DV §7.1 proíbem ("falso positivo é aceitável; vazamento não"). Cada remoção está
// justificada no comentário do próprio padrão.
var rules = []rule{
	{"private_key", regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\z)`)},
	{"url_credentials", regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s/:@]+:[^\s/@]+@`)},
	// Sem `\b`: os nomes de cabeçalho são longos e específicos; `X-Api-Key:` colado em letras casa.
	{"auth_header", regexp.MustCompile(`(?i)(?:authorization|x-api-key|private-token|ocp-apim-subscription-key|x-amz-security-token|x-auth-token|api-key)\s*:\s*[^\r\n]+`)},
	// Sem `\b`: `bearer` + espaço + 16 ou mais caracteres de token já é específico.
	{"bearer_token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=\-]{16,}`)},
	// Sem `\b`: `sk-` + 16 ou mais caracteres. Falso positivo em identificadores como `task-...` longo é aceitável.
	{"api_key", regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`)},
	// Sem `\b`: `sk_live_` / `rk_live_` é prefixo específico.
	{"stripe_key", regexp.MustCompile(`[sr]k_live_[A-Za-z0-9]{8,}`)},
	// Sem `\b`: `gh[pousr]_` e `github_pat_` são prefixos específicos.
	{"github_token", regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`)},
	// Sem `\b` no início nem no fim. `AKIA`/`ASIA` + 16 caracteres é tamanho fixo (20 no total).
	// O `\b` final deixava escapar `AKIA<16>` seguido de minúscula (`X` e `a` são ambos caracteres
	// de palavra, sem fronteira). Sem ele, o segredo de 20 caracteres sempre é redigido; a letra
	// que vem depois sobra como texto, o que é falso positivo aceitável. `ASIAPACIFICREGION` tem
	// só 13 caracteres depois de `ASIA`, então não casa.
	{"aws_key", regexp.MustCompile(`(?:AKIA|ASIA)[A-Z0-9]{16}`)},
	// Sem `\b`: `AIza` e `AQ.Ab` são prefixos específicos.
	{"google_api_key", regexp.MustCompile(`AIza[A-Za-z0-9_\-]{26,}|AQ\.Ab[A-Za-z0-9_\-]{28,}`)},
	// Sem `\b`: `1//` + 17 ou mais caracteres. Falso positivo em `x1//...` longo é aceitável.
	{"google_oauth", regexp.MustCompile(`1//[A-Za-z0-9_\-]{17,}`)},
	// Sem `\b`: `EAA` + 20 ou mais alfanuméricos.
	{"meta_token", regexp.MustCompile(`EAA[A-Za-z0-9]{20,}`)},
	// Sem `\b`, desvio do critério de prefixo: este padrão não tem prefixo literal, só dígitos,
	// `:` e 30 ou mais caracteres. Com `\b`, `abc123456789:<token>` escapava. A fronteira foi
	// removida para cobrir o caso colado; o custo é falso positivo em dígitos colados a letras.
	{"telegram_token", regexp.MustCompile(`[0-9]{6,10}:[A-Za-z0-9_\-]{30,}`)},
	// Sem `\b` no início nem no fim. `pit-` é prefixo específico e o UUID tem tamanho fixo.
	// O `\b` final deixava escapar `<uuid>g` (letra colada depois do UUID).
	{"ghl_token", regexp.MustCompile(`pit-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)},
	// Sem `\b`: `xox[baprs]-` e `xapp-` são prefixos específicos.
	{"slack_token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}|xapp-[A-Za-z0-9\-]{10,}`)},
	// Sem `\b`: `eyJ` (início de JSON em base64) é prefixo específico.
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`)},
	// Sem `\b`: `eyJ` é prefixo específico.
	{"base64_json_token", regexp.MustCompile(`eyJ[A-Za-z0-9_\-+/=]{37,}`)},
	// Sem `\b`: `GITHUB_TOKEN=` colado em letras (`aGITHUB_TOKEN=...`) casaria a partir do `G`.
	// Nomes de variável em maiúsculas com sufixo de segredo são específicos. `MAX_TOKENS=100` não
	// casa porque o sufixo exige `_TOKEN` seguido de `=` ou `:`, e `_TOKENS` não é o sufixo.
	{"env_secret", regexp.MustCompile(`[A-Z][A-Z0-9_]*(?:_KEY|_TOKEN|_SECRET|_PASSWORD|_PASSWD|_CREDENTIALS?|_PRIVATE_KEY|_KEY_ID|_PASSPHRASE|_SIGNING_KEY|_PEPPER|_SALT)\s*[=:]\s*\S+`)},
	{"credential_path", regexp.MustCompile(`(?i)(?:[a-z]:|~)?[\\/][^\s"']*?(?:\.ssh|\.aws|\.kube|\.config[\\/]gcloud|\.gnupg)(?:[\\/][^\s"']*)?`)},
}

// redact aplica os padrões em ordem. Cada trecho casado vira `[REDACTED:<rótulo>]`.
func redact(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllStringFunc(s, func(match string) string {
			return "[REDACTED:" + r.label + "]"
		})
	}
	return s
}
