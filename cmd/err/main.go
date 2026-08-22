// err (errata) — a personal memory for terminal errors.
//
// The binary is named "err"; the module/product is errata. Living under
// cmd/err makes `go install github.com/wumuwutu/errata/cmd/err@latest`
// produce an `err` binary.
package main

import "github.com/wumuwutu/errata/internal/cli"

func main() {
	cli.Execute()
}
