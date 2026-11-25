# Multihttp scenarios

Each file is one multihttp check, the replies of a local fake server, and what a
run must produce. `TestScenarios` in `../../scenario_test.go` runs every file
against each k6 binary in `dist/`.

Each file covers one multihttp check shape, set by the code paths a check takes
rather than by what it monitors:

- whether there is one step or several;
- whether a request has a body or query fields;
- which assertion kinds it uses;
- which variable kinds it saves;
- where a saved value is used again: URL, header, query field, body, assertion
  value, or the whole URL;
- whether a step asks for a compressed reply;
- where a run stops: a failed assertion, a saved value that matched nothing, or
  a reply that a variable cannot read.

Shapes that differ only in method, host, step count or values share a file. Each
file was written by hand and adds the request details that matter to its shape,
such as a Host or Content-Type header, a URL with escapes, or a JSONPath without
the leading `$`. Every value is invented, and every request goes to the fake
server.

## Format

- `settings` is the check's multihttp settings as the API encodes them.
  Assertions live under `checks`, enums are numbers (only `method` is a string),
  and body payloads are base64.
- `routes` maps `METHOD /path` (the decoded path) to a reply: `status`,
  `headers`, `body` and `gzip`. A request with no route gets a 404.
- `expect` lists the overall result, the assertion pass and fail totals, and the
  requests the server must see, in order. Each request can list header, query
  and body parts it must contain.
- `{{base}}` and `{{alt}}` stand for the fake server and a second address of it.

Unknown keys fail the file, and `settings` must pass validation.

## Adding and running a scenario

Add a kebab-case `<name>.json` here, then run it on Linux, after `make sm-k6-native`
has put the binaries in `dist/` (elsewhere the test skips):

```sh
go test -run 'TestScenarios/<name>' ./internal/prober/multihttp/
```
