// Command ai-memory é o binário da CLI. Só delega a internal/cli; não há lógica aqui.
package main

import (
	"os"

	"github.com/phm-aguiar/ai-memory-go/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
