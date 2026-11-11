# Recipes stay on the intersection of pwsh and sh syntax, because this project
# builds on Windows and on Linux.
#
# Windows has no sh on PATH by default, so only Windows gets an override.
set windows-shell := ["pwsh.exe", "-NoProfile", "-Command"]

# Builds and checks the formatter.
default:
    @just --list

# Builds the command for this machine.
build:
    go build -trimpath -ldflags "-s -w" -o bin/sqol ./...

# Runs the tests.
test:
    go test ./...

# Reports vet problems and a file an editor left unformatted.
lint:
    go vet ./...
    go test -run TestTheTreeIsGofmtClean ./...

# Fails when a file would change, so a CI job can run it as a gate.
check:
    go run ./... -l .

# Builds the five binaries the editor extension ships, named by Go target.
binaries: binary-windows-amd64 binary-linux-amd64 binary-linux-arm64 binary-darwin-amd64 binary-darwin-arm64
    @echo "binaries ready"

[env('GOOS', 'windows')]
[env('GOARCH', 'amd64')]
[env('CGO_ENABLED', '0')]
binary-windows-amd64:
    go build -trimpath -ldflags "-s -w" -o editors/vscode/bin/sqol-windows-amd64.exe ./...

[env('GOOS', 'linux')]
[env('GOARCH', 'amd64')]
[env('CGO_ENABLED', '0')]
binary-linux-amd64:
    go build -trimpath -ldflags "-s -w" -o editors/vscode/bin/sqol-linux-amd64 ./...

[env('GOOS', 'linux')]
[env('GOARCH', 'arm64')]
[env('CGO_ENABLED', '0')]
binary-linux-arm64:
    go build -trimpath -ldflags "-s -w" -o editors/vscode/bin/sqol-linux-arm64 ./...

[env('GOOS', 'darwin')]
[env('GOARCH', 'amd64')]
[env('CGO_ENABLED', '0')]
binary-darwin-amd64:
    go build -trimpath -ldflags "-s -w" -o editors/vscode/bin/sqol-darwin-amd64 ./...

[env('GOOS', 'darwin')]
[env('GOARCH', 'arm64')]
[env('CGO_ENABLED', '0')]
binary-darwin-arm64:
    go build -trimpath -ldflags "-s -w" -o editors/vscode/bin/sqol-darwin-arm64 ./...

# Compiles and checks the editor extension.
extension:
    cd editors/vscode && corepack pnpm install
    cd editors/vscode && corepack pnpm exec biome check .
    cd editors/vscode && corepack pnpm exec tsc -p .

# Packages the extension, which needs the binaries first.
package: binaries extension
    cd editors/vscode && corepack pnpm exec vsce package
