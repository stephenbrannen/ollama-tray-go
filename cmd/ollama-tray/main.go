// Command ollama-tray shows which models an Ollama server has loaded in the system tray.
package main

import (
	"fmt"

	"github.com/stephenbrannen/ollama-tray-go/internal/version"
)

func main() {
	fmt.Println(version.String())
}
