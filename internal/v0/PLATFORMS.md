# Supported worker platforms

The first release builds the worker for these targets:

| OS | Architecture | Go target | Release status |
|---|---|---|---|
| Linux | amd64 (x86_64) | `linux/amd64` | Supported |
| Linux | arm64 (aarch64) | `linux/arm64` | Supported |
| Windows | amd64 (x86_64) | `windows/amd64` | Supported |

The release pipeline must compile and test each supported target. Windows
arm64 and other operating systems remain deferred until a deployment need is
identified.
