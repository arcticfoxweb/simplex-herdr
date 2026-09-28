# simplex

Agents send and receive [SimpleX](https://simplex.chat) messages and files. One profile is one agent: a SimpleX identity, a local `simplex-chat` process, and an inbox. When that agent's [Herdr](https://herdr.dev) pane is idle, a new message is submitted there. The agent replies with `send`.

The same binary is the shell command, the worker for the Herdr plugin `simplex.agents`, and a stdio MCP server. Any agent that can run a program or start an MCP server can use it.

## Install

The current release is the alpha [v0.1.0-alpha.1](https://github.com/arcticfoxweb/simplex-herdr/releases/tag/v0.1.0-alpha.1). Herdr 0.7 or newer is required if messages should land in a pane. This was developed against Herdr 0.9.

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.sh | sh
```

Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.ps1 | iex
```

The Windows one-liner downloads `simplex-windows-amd64.exe` from the latest release, checks it against `SHA256SUMS`, and does not need Go. Windows is amd64 only, because that is the official `simplex-chat` build. Set `SIMPLEX_FROM_SOURCE=1` to compile instead. Linux and macOS one-liners do the same for their release binary, and build from source if the release asset is missing.

Install Herdr before this installer, or run the installer again after Herdr. Herdr's installer writes its own `PATH` entry. If Simplex was installed first, an agent pane will not see `simplex` until `simplex.cmd` is placed next to `herdr.exe`. Running the installer again does that and links the plugin.

From a checkout of this tree, the install script compiles with Go:

```sh
./install.sh
```

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

A source build needs Go 1.22 or newer. The module uses Go 1.26, so an older toolchain downloads it. The installer puts `simplex` in `~/.local/bin` (`%USERPROFILE%\.local\bin\simplex.exe` on Windows), downloads the official `simplex-chat` binary when it is missing, and links the Herdr plugin when `herdr` is on `PATH`. On Windows it also adds that directory to the user Path. On Linux and macOS, `~/.local/bin` has to be on `PATH` already. Open a new terminal if `simplex` is not found. Restart Herdr once after the first install so the plugin startup hook runs. Linking a plugin does not start that hook by itself.

This release does not include `simplex-chat`. That program is SimpleX's own build. `simplex install` downloads it and prints the sha256. Compare that hash with the [SimpleX release notes](https://github.com/simplex-chat/simplex-chat/releases/latest) before trusting the binary.

| OS | Arch | Release asset |
| --- | --- | --- |
| Linux | amd64 | `simplex-chat-ubuntu-24_04-x86_64` |
| Linux | arm64 | `simplex-chat-ubuntu-24_04-aarch64` |
| macOS | arm64 | `simplex-chat-macos-aarch64` |
| macOS | amd64 | `simplex-chat-macos-x86-64` |
| Windows | amd64 | `simplex-chat-windows-x86-64` |

On Windows, use the PowerShell one-liner above. `herdr plugin install` is the path for a machine that already has Go. On Windows that build runs `build.ps1`, which needs Go. Linux and macOS run `build.sh`.

```sh
herdr plugin install arcticfoxweb/simplex-herdr/herdr-plugin --yes
```

`herdr plugin link` does not compile. `install.sh` and `install.ps1` install the release binary and link the plugin.

The Windows `simplex-chat` program imports `libcrypto-3-x64.dll`. The installer places that DLL beside `simplex-chat.exe`, from the FireDaemon OpenSSL 3.0.22 build, and launches the program. If it still exits `0xc0000135`, the installer adds `libssl-3-x64.dll` and tries again. A second `0xc0000135` fails the install instead of leaving a chat binary that crash-loops.

## Two agents

```sh
simplex init alice --pane w1:p1
simplex qr

simplex init bob --pane w3:p1
simplex --profile bob connect "$(simplex --profile alice address)"
```

`w1:p1` is the Herdr pane where Alice's agent is already running. `simplex qr` draws a small square QR of the short `https://` contact link, not the long `simplex:/contact…` address. The address is a capability: anyone who has it can message that agent.

Send from the shell:

```sh
simplex --profile alice send bob "hello"
simplex --profile alice send-file bob ./notes.txt
```

Send through the Herdr plugin. Actions take no extra arguments, so the recipient and the text are environment variables:

```sh
herdr plugin pane open --plugin simplex.agents --entrypoint send \
  --env SIMPLEX_TO=bob \
  --env SIMPLEX_TEXT='hello'
```

The send pane closes after a successful send. `SIMPLEX_TO` is a contact name or a group name. The shell command and the plugin call the same daemon operation.

`simplex send-file` attaches a file to a text message. The SimpleX app shows that as a file row. It does not draw an inline picture. The full agent instructions are `simplex help` and [herdr-plugin/SKILL.md](herdr-plugin/SKILL.md). `simplex plugin status` prints them after the daemon status.

When Bob's pane is idle, the daemon submits:

```text
alice says [msg:direct:2:10]: hello
file: ~/.config/simplex/profiles/bob/files/notes.txt
```

Bob replies with `send`. Both turns show up in the panes.

## Groups

A group you have already joined is a name:

```sh
simplex send Agents "hello"
simplex send "#Agents" "hello"
```

A leading `#` forces a group when a contact uses the same name. This version has no command to create a group, invite members, or join one. Accept the invitation in SimpleX Chat, then send by the group name.

## Herdr plugin

Plugin id `simplex.agents`. Manifest: `herdr-plugin/herdr-plugin.toml`.

Each action starts `simplex` and exits. The daemon is what stays connected. `attach` and the startup hook make sure that daemon is up and pointed at a pane. Incoming text is submitted into the pane. It does not show up in the plugin log.

From the pane that should receive messages:

```sh
herdr plugin action invoke simplex.agents.attach
herdr plugin action invoke simplex.agents.status
```

## Delivery

The daemon submits the oldest unread message with `herdr agent prompt` only when Herdr reports that agent `idle` or `done` and the visible screen has stayed still. It does not type over output, or over a line the agent is still writing.

By default that is the next stable idle look. To wait after the screen settles:

```sh
simplex gateway w1:p1 --quiet 2s
```

The minimum is 200ms. `simplex gateway` with no arguments prints the current pane. `simplex gateway off` stops delivery into the pane.

A direct line looks like `NAME says [msg:direct:<contactId>:<itemId>]: text`. A group line looks like `<speaker> says in <group> [msg:group:<groupId>:<itemId>]: text`. A `file:` line is added when a real local path exists.

```sh
simplex inbox
simplex inbox --wait 30s
simplex inbox --all
simplex ack msg:direct:2:10
```

`inbox` lists what is still unread. `--all` includes messages already delivered or acked. `--wait` blocks until something is unread.

Incoming voice and video calls are rejected and recorded as seen. They are not typed into the pane. Join notices, encryption banners, and group setting events are not messages.

## MCP

Any agent that can launch a stdio MCP server, one process per profile:

```toml
command = "simplex"
args = ["mcp", "--profile", "alice"]
```

Tools: `address`, `connect`, `contacts`, `send`, `send_file`, `inbox`, `ack`.

`send` and `send_file` take a contact or a group name. Prefix a group with `#` when it shares a name with a contact. After a message has been typed into the pane, reply once, then `ack` the id.

Installing simplex does not register instructions in an agent's skill directory. The commands and these tools are the interface.

## Files

Incoming files are accepted automatically, up to 100MB, into the profile `files` directory. The inbox entry includes the local path once the download finishes.

| Path | What |
| --- | --- |
| `~/.config/simplex/profiles/<name>/` | Linux and macOS profile, database, inbox, and files. |
| `%APPDATA%\simplex\profiles\<name>\` | Windows profile. That is `C:\Users\<you>\AppData\Roaming\simplex\profiles\<name>`. |
| `~/.local/share/simplex/bin/simplex-chat` | Official binary from `simplex install`. Override with `SIMPLEX_CHAT_BIN`. |
| `~/.local/bin/simplex` | This CLI. On Windows, `%USERPROFILE%\.local\bin\simplex.exe`. |
| `~/.local/src/simplex` | Checkout used by the piped installer. The Herdr plugin link points here, so leave it in place. |

`SIMPLEX_HOME` overrides the profile root on every system.

`simplex status` shows the daemon, the address, the unread count, and the Herdr pane. `simplex down` stops the daemon. `simplex up` runs it in the foreground.

On Windows the control endpoint is the named pipe `\\.\pipe\simplex-<profile>`. On Linux and macOS it is a Unix socket inside the profile directory.

## Commands

```text
simplex install
simplex init [name] [--pane PANE]
simplex use <name>
simplex address
simplex qr
simplex connect <link>
simplex contacts
simplex send <contact-or-group> <text>
simplex send-file <contact-or-group> <path> [caption]
simplex inbox [--wait 30s] [--all] [--ack]
simplex ack <id>...
simplex gateway [PANE | off] [--quiet 2s]
simplex status
simplex profiles
simplex up
simplex down
simplex mcp
simplex version
```

`--profile <name>` selects a profile. `SIMPLEX_PROFILE` does the same. `--json` prints the raw result. `simplex send NAME -` reads the message from stdin.

## Limits

- No command to create a group, invite members, or join from the CLI.
- No voice or video. Incoming calls are rejected.
- No license file. Nothing in this tree grants one.
- Linux is the only host where sending and receiving have been run. The macOS and Windows binaries compile. They have not been run, and two machines have not messaged each other.
- The official Windows `simplex-chat` asset is amd64. Herdr plugins on Windows are a preview. `install.ps1` has been syntax-checked and its checkout path has been run under PowerShell on Linux. It has not been executed on Windows.

## Development

```sh
go test ./...
```

Cross builds, not run on those hosts:

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/simplex-darwin-arm64 ./cmd/simplex
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/simplex-windows-amd64.exe ./cmd/simplex
```

`dist/` is gitignored.
