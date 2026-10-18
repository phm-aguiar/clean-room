package wiki

// logFileName é o nome do ledger bruto de eventos de hook (docs/wiki-files-layer.md §9). O ai-memory
// não escreve log.md: o histórico do wiki é o próprio histórico de commits (docs/okf.md). O nome
// continua aqui porque o reconhecimento de ledger (ledger_detect.go) o usa.
const logFileName = "log.md"
