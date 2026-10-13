//go:build !race

package bench

// detectorRaceAtivo é falso quando o pacote é compilado sem -race (ver race_on_test.go).
const detectorRaceAtivo = false
