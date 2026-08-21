// err (dejavu) — a personal memory for terminal errors.
//
// The binary is named "err"; the module/product is dejavu. Living under
// cmd/err makes `go install github.com/wumuwutu/dejavu/cmd/err@latest`
// produce an `err` binary.
package main

import "github.com/wumuwutu/dejavu/internal/cli"

func main() {
	cli.Execute()
}
