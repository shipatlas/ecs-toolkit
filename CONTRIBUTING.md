# Contributing

## Development

Below instructions are only necessary if you intend to work on the source code.
For normal usage the above instructions should do.

### Requirements

1. Ensure that you have a [properly configured][golang-quickstart] Go workspace.

### Building

1. Clone the repository.
2. Fetch the dependencies with `go get -v github.com/shipatlas/ecs-toolkit`.
4. Install application dependencies via `make dependencies` (they'll be placed
   in `./vendor`).
5. Build and install the binary with `make build`.
6. Run the command e.g. `./bin/ecs-toolkit help` as a basic test.

### Testing

1. Install the `golangci-lint`, [see instructions here][golangci-lint-install].
2. Run linter using `make lint` and test using `make test`.

[golang-quickstart]: https://go.dev/doc/tutorial/getting-started
[golangci-lint-install]: https://golangci-lint.run/usage/install/
