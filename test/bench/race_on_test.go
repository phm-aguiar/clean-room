//go:build race

package bench

// detectorRaceAtivo é verdadeiro quando o pacote é compilado com -race. O detector deixa a
// execução várias vezes mais lenta, então o teste de tempo não é válido sob ele.
const detectorRaceAtivo = true
