## Golang Guidelines

- To see source files from a dependency, or to answer questions about a dependency, run `go mod download -json MODULE` and use the returned `Dir` path to read the files.
- Use `go doc foo.Bar` or `go doc -all foo` to read documentation for packages, types, functions, etc.
- Use `go run .` or `go run ./cmd/foo` instead of `go build` to run programs, to avoid leaving behind build artifacts.
- Use `any` instead of `interface{}`.
- Use `cmp.Or(val, fallback)` instead of `if val == zero { val = fallback }` for defaulting zero values.
- Use `go fix` after each code change.
- Use `github.com/goccy/go-yaml` instead of `gopkg.in/yaml.v3`
- Use `make fmt` to format the source files.
