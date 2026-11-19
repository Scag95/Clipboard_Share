# Windows testing without a Windows machine
Test the Nudge Windows build (branch feat/windows-build, base db40efb) without a physical Windows device: locally under Wine and on real Windows through GitHub Actions `windows-latest`. Fix what the tests reveal.

## Specs
- S1: The user asked: "Alguna otra manera de hacerlo en windows sin tener windows?" and then chose the recommendation: "Vamos con tu recomendación. sudo no te va  pedir contraseña por 15 minutos. adelante." The recommendation was "1 + 2": Wine on the local Linux for fast iteration, plus GitHub Actions `windows-latest` for real-Windows checks of what Wine cannot cover.
- S2 (Wine): install Wine; run the Go test suite compiled for Windows under Wine (`GOOS=windows go test -exec wine ./...`); smoke-test `nudge.exe` under Wine: server starts, HTTP answers on 8765, the log/config dir is created, `nudge pair` CLI talks to the control socket (or the documented fallback), the clipboard round-trip works. Report which failures are Wine limitations and which are real bugs.
- S3 (Actions): add a GitHub Actions workflow that on Windows runs `go vet` and `go test ./...`, builds both executables, starts `nudge.exe`, checks HTTP and the control socket/CLI, and runs `install.ps1` / `uninstall.ps1` non-interactively. It runs on pushes to the feature branch and on pull requests.
- S4: Real bugs found are fixed with a test where one is meaningful; Linux behavior unchanged (`go build ./... && go vet ./... && go test ./...` still pass on Linux).

## Tasks
- T1 (S2) route: inline — install Wine, run Windows tests under Wine, smoke test. Commit: n/a (no source change unless bugs).
- T2 (S3) route: inline — .github/workflows/windows.yml; push the feature branch and watch the run. Commit: pending.
- T3 (S4) route: inline — fix bugs found by T1/T2. Commit: pending.

## Log
- L1: Vamos a ahora con la app de windows, tengo un ordenador con windows. podemos conectarnos por ssh y puedas hacer pruebas con eso?
- L2: Alguna otra manera de hacerlo en windows sin tener windows?
- L3: Vamos con tu recomendación. sudo no te va  pedir contraseña por 15 minutos. adelante.
