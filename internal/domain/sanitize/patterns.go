package sanitize

import "regexp"

// rule é um padrão de segredo com o rótulo que aparece no marcador `[REDACTED:<rótulo>]`.
type rule struct {
	label string
	re    *regexp.Regexp
}

// builtinRules são os 27 padrões de docs/prompts-schemas-and-sanitizer.md §11.2, na ordem da tabela.
// A ordem importa: um trecho redigido por um padrão anterior não chega aos seguintes (§11.1, passo 3).
//
// Todos são RE2 (`regexp` do Go): sem lookaround nem referência a grupo. A borda de palavra `\b`
// aparece só onde a tabela a exige (`aws_key`, `telegram_token`, `auth_header` e `env_secret`
// de variável em maiúsculas); os demais prefixos são específicos e casam colados em letras.
var builtinRules = []rule{
	{"bearer_token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-+/=]{16,}`)},
	{"api_key", regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`)},
	{"stripe_key", regexp.MustCompile(`(?:sk|rk)_live_[A-Za-z0-9_\-]{16,}`)},
	{"github_token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{"github_token", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{"aws_key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"google_api_key", regexp.MustCompile(`AIza[A-Za-z0-9_\-]{30,}`)},
	{"google_api_key", regexp.MustCompile(`AQ\.Ab[A-Za-z0-9_\-]{30,}`)},
	{"google_oauth", regexp.MustCompile(`1//[0-9A-Za-z_\-]{20,}`)},
	{"meta_token", regexp.MustCompile(`EAA[A-Za-z0-9]{20,}`)},
	{"telegram_token", regexp.MustCompile(`\b\d{6,10}:(?:AA[A-Za-z0-9_\-]{30,}|[A-Za-z0-9_\-]{34,35})\b`)},
	{"ghl_token", regexp.MustCompile(`pit-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)},
	{"slack_token", regexp.MustCompile(`xox[abprs]-[A-Za-z0-9\-]{10,}`)},
	{"slack_token", regexp.MustCompile(`xapp-[A-Za-z0-9\-]{10,}`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}`)},
	{"base64_json_token", regexp.MustCompile(`eyJ[A-Za-z0-9_\-+/]{40,}={0,2}`)},
	{"private_key", regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)},
	{"url_credentials", regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+\-.]*://[^:/\s]+:[^@\s]+@[^\s]+`)},
	{"auth_header", regexp.MustCompile(`(?i)\b[A-Za-z0-9_-]*(?:authentication|authorization|credentials?|password|passwd|apikey|accountkey|authtoken|[a-z0-9]*(?:api|auth|access|secret|security|private|session|refresh|client|consumer|subscription|app|bearer)-(?:key|token))"?\s*[=:]\s*(?:(?:basic|bearer|digest|token|apikey)\s+)?"?[A-Za-z0-9._~+/=-]{8,}`)},
	{"env_secret", regexp.MustCompile(`(?i)(?:ANTHROPIC_API_KEY|OPENAI_API_KEY|OPENROUTER_API_KEY|VOYAGE_API_KEY|MISTRAL_API_KEY|GROQ_API_KEY|HF_TOKEN|HUGGINGFACE_TOKEN|AWS_(?:SECRET_)?ACCESS_KEY[A-Z_]*|GITHUB_TOKEN|GH_TOKEN|GITLAB_TOKEN|GOOGLE_API_KEY|GEMINI_API_KEY|OLLAMA_API_KEY)"?\s*[=:]\s*\S+`)},
	{"env_secret", regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9_]*_(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIALS?|PRIVATE_KEY)"?\s*[=:]\s*\S+`)},
	{"env_secret", regexp.MustCompile(`\b[A-Z][A-Z0-9_]*_(?:KEY_ID|PASSPHRASE|SIGNING_KEY|PEPPER|SALT)"?\s*[=:]\s*\S+`)},
	{"credential_path", regexp.MustCompile(`(?i)(?:[A-Za-z]:)?(?:[\\/][^\\/\s]+)*[\\/]\.ssh(?:[\\/][^\s]+)?`)},
	{"credential_path", regexp.MustCompile(`(?i)(?:[A-Za-z]:)?(?:[\\/][^\\/\s]+)*[\\/]\.aws(?:[\\/][^\s]+)?`)},
	{"credential_path", regexp.MustCompile(`(?i)(?:[A-Za-z]:)?(?:[\\/][^\\/\s]+)*[\\/]\.kube(?:[\\/][^\s]+)?`)},
	{"credential_path", regexp.MustCompile(`(?i)(?:[A-Za-z]:)?(?:[\\/][^\\/\s]+)*[\\/]\.config[\\/]gcloud(?:[\\/][^\s]+)?`)},
	{"credential_path", regexp.MustCompile(`(?i)(?:[A-Za-z]:)?(?:[\\/][^\\/\s]+)*[\\/]\.gnupg(?:[\\/][^\s]+)?`)},
}
