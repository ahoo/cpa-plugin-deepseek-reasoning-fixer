# cpa-plugin-deepseek-reasoning-fixer

CLIProxyAPI request normalizer plugin for DeepSeek thinking-mode conversations.

When clients (e.g. Codex CLI, deepseek-harness) run long agentic sessions against
DeepSeek with `reasoning_effort` != `none`, they sometimes drop the
`reasoning_content` field on individual assistant messages (missing key, JSON
`null`, empty string, or empty array). The DeepSeek Console upstream enforces
thinking mode strictly and rejects such requests with:

```text
400 invalid_request_error: The `reasoning_content` in the thinking mode must be passed back to the API.
```

This plugin normalizes outgoing requests: for DeepSeek models in thinking mode
it fills missing/empty `reasoning_content` on assistant messages with a
placeholder, so the upstream accepts the request and the conversation survives.

## Capability

- `request_normalizer` — runs on every translated request.
- Only touches requests whose model name contains `deepseek` and whose
  `reasoning_effort` is set to something other than `none`.
- Assistant messages with a valid non-empty `reasoning_content` are left
  untouched; unknown shapes (objects, numbers, booleans) are passed through
  verbatim.

## Install

Add the store source and enable the plugin in `config.yaml`:

```yaml
plugins:
  enabled: true
  store-sources:
    - https://raw.githubusercontent.com/ahoo/cpa-plugin-deepseek-reasoning-fixer/main/registry.json
  configs:
    deepseek-reasoning-fixer:
      enabled: true
      priority: 1
```

Then restart CLIProxyAPI (plugins are loaded at startup):

```bash
docker restart cli-proxy-api
```

Verify the plugin registered:

```bash
docker logs cli-proxy-api | grep reasoning-fixer
```

## Build

The CLIProxyAPI runtime image is Debian-based (glibc). Building with the
alpine Go image produces a musl-linked `.so` that fails to `dlopen` at runtime
(`libc.musl-x86_64.so.1: cannot open shared object file`). Use a glibc image:

```bash
./build.sh
# or manually:
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 docker run --rm \
  -v "$PWD:/src" -w /src golang:1.26-bookworm \
  go build -buildmode=c-shared -o deepseek-reasoning-fixer.so .
```

Output goes to `../../plugins/linux/amd64/` relative to the repo checkout,
matching the CLIProxyAPI plugins bind mount layout.

## Test

```bash
go vet . && go test ./...
```

## Release

1. Build the `.so`.
2. Package `<id>_<version>_<goos>_<goarch>.zip` with the library at the zip
   root named `deepseek-reasoning-fixer.so`.
3. Generate `checksums.txt` (sha256 of the zip).
4. `gh release create v0.1.1 deepseek-reasoning-fixer_0.1.1_linux_amd64.zip checksums.txt`

## License

MIT
