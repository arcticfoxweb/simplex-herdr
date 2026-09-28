# simplex

A command-line agent messenger on [SimpleX](https://simplex.chat). One profile is one agent: its own SimpleX identity, a local `simplex-chat` process, and an inbox. Incoming text is submitted into that agent's [Herdr](https://herdr.dev) pane when the pane is idle. The agent replies with `send`.

The same binary is the shell command, the Herdr plugin's worker, and a stdio MCP server. Any agent that can run a program or an MCP server can use it. It is not tied to one vendor.

## Install

From this checkout:

```sh
./install.sh
```

That builds `simplex` into `~/.local/bin`, downloads the official `simplex-chat` binary when it is not already installed, and links the Herdr plugin when `herdr` is on `PATH`.

Once this tree is published, the same script is the one-line install. The default clone URL is `https://github.com/arcticfoxweb/simplex.git`. Override it with `SIMPLEX_REPO_URL`.

```sh
curl -fsSL https://raw.githubusercontent.com/arcticfoxweb/simplex/main/install.sh | sh
```

The pipe works only after that repository exists. Until then, run `./install.sh` from a checkout.

`~/.local/bin` has to be on `PATH`. The first install should be followed by a Herdr restart so the plugin startup hook runs. Linking a plugin does not start that hook by itself.

A published checkout can also be installed as a Herdr plugin. On Linux and macOS the plugin build compiles `simplex` and downloads `simplex-chat` if needed:

```sh
herdr plugin install arcticfoxweb/simplex/herdr-plugin --yes
```

`herdr plugin link` does not run that build. `./install.sh` does.

### Requirements

- Go 1.22 or newer. The module asks for Go 1.26; an older toolchain downloads it.
- `git`, for the piped installer.
- Herdr 0.7 or newer, for pane delivery. This machine uses 0.9.
- An official `simplex-chat` build for the host:

| OS | Arch | Release asset |
| --- | --- | --- |
| Linux | amd64 | `simplex-chat-ubuntu-24_04-x86_64` |
| Linux | arm64 | `simplex-chat-ubuntu-24_04-aarch64` |
| macOS | arm64 | `simplex-chat-macos-aarch64` |
| macOS | amd64 | `simplex-chat-macos-x86-64` |
| Windows | amd64 | `simplex-chat-windows-x86-64` |

`simplex install` prints the sha256. Compare it with the [SimpleX release notes](https://github.com/simplex-chat/simplex-chat/releases/latest) before trusting the binary.

Linux is the host this program has actually sent and received on. The macOS and Windows `simplex` binaries compile. They have not been run, and two machines have not messaged each other. Herdr plugins on Windows are still a preview.

On Windows, build with Go, run `simplex install`, put `simplex.exe` on `PATH`, and link `herdr-plugin`. The plugin build script is skipped there because it is a POSIX shell script.

## Two agents

```sh
simplex init alice --pane w1:p1
simplex qr

simplex init bob --pane w3:p1
simplex --profile bob connect "$(simplex --profile alice address)"
```

`w1:p1` is the Herdr pane where Alice's CLI is already running. `simplex qr` draws a small square QR of the short contact link (`https://…`), not the long `simplex:/contact…` address. Scan that. The address is a capability: anyone who has it can message that agent.

```sh
simplex --profile alice send bob "hello"
simplex --profile alice send-file bob ./notes.txt
```

When Bob's pane is idle, the daemon submits:

```text
alice says [msg:direct:2:10]: hello
file: /home/you/.config/simplex/profiles/bob/files/notes.txt
```

Bob replies with `simplex send` (or the plugin command below). You see both turns in the panes.

A group you have already joined is a name, the same as a contact:

```sh
simplex send "Tangled Development" "hello"
simplex send "#Tangled Development" "hello"
```

A leading `#` forces a group when a contact uses the same name. There is no `simplex join` or `simplex groups` yet. Accept the invitation in SimpleX, then send by the group name.

## Herdr plugin

Plugin id: `simplex.agents`. Manifest: `herdr-plugin/herdr-plugin.toml`.

The plugin is the front door inside Herdr. Each action starts `simplex`, prints a result, and exits. The long-running process is still the per-profile daemon; the startup hook and `attach` make sure that daemon is up. Chat text shows up in the agent pane, not in the plugin log.

Attach the pane you are in:

```sh
herdr plugin action invoke simplex.agents.attach
```

Status:

```sh
herdr plugin action invoke simplex.agents.status
```

Send. Plugin actions take no extra arguments, so the recipient and text are environment variables:

```sh
herdr plugin pane open --plugin simplex.agents --entrypoint send \
  --env SIMPLEX_TO=bob \
  --env SIMPLEX_TEXT='hello'
```

The send pane closes after a successful send. `SIMPLEX_TO` is a contact name or a group name.

Shell `simplex send` and this pane call the same daemon operation. Use whichever the agent can run.

## When a message is delivered

The daemon submits the oldest due inbox message with `herdr agent prompt` only when Herdr reports that agent `idle` or `done` and the visible screen has stayed still. It does not type over output or over a line the agent is still writing.

The default quiet time is immediate: the next stable idle look. To wait longer after the screen settles:

```sh
simplex gateway w1:p1 --quiet 2s
```

The minimum is 200ms. `simplex gateway` with no arguments prints the current pane.

A direct line looks like `NAME says [msg:direct:<contactId>:<itemId>]: text`. A group line looks like `<speaker> says in <group> [msg:group:<groupId>:<itemId>]: text`. A `file:` line is added when a real local path exists.

After the agent has dealt with the message:

```sh
simplex ack msg:direct:2:10
```

`simplex inbox` lists what is still unread. `simplex inbox --all` includes messages already delivered or acked. `simplex inbox --wait 30s` blocks until something is unread.

Incoming voice and video calls are rejected and recorded as seen. They are not typed into the pane. This build does not take calls.

Join notices, encryption banners, and group feature events are not messages and are not submitted.

## MCP

Any agent that can launch a stdio MCP server, one process per profile:

```toml
command = "simplex"
args = ["mcp", "--profile", "alice"]
```

Tools: `address`, `connect`, `contacts`, `send`, `send_file`, `inbox`, `ack`.

`send` and `send_file` take a contact or a group name. Prefix a group with `#` when it shares a name with a contact. `inbox` marks a message delivered once it has been typed into the pane. Reply once, then `ack` the id.

Installing the CLI does not register these instructions inside each agent. The commands and the MCP tools are the interface.

## Files and layout

Incoming files are accepted automatically, up to 100MB, into the profile `files` directory. The inbox entry includes the local path once the download finishes.

| Path | What |
| --- | --- |
| `~/.config/simplex/profiles/<name>/` | Profile, database, inbox, files. Override the root with `SIMPLEX_HOME`. |
| `~/.local/share/simplex/bin/simplex-chat` | Official binary from `simplex install`. Override with `SIMPLEX_CHAT_BIN`. |
| `~/.local/bin/simplex` | This CLI. |
| `~/.local/src/simplex` | Checkout used by the piped installer. The Herdr plugin link points here, so leave it in place. |

`simplex status` shows the daemon, the address, unread count, and the Herdr pane. `simplex down` stops the daemon. `simplex up` runs it in the foreground.

On Windows the control endpoint is the named pipe `\\.\pipe\simplex-<profile>`. Everywhere else it is a Unix socket inside the profile directory.

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

`--profile <name>` selects a profile. `SIMPLEX_PROFILE` does the same. `--json` prints the raw result.

`simplex send NAME -` reads the message from stdin.

## What this version does not do

- Create a group, invite members, or join from the CLI.
- Voice or video.
- Teach an agent how to call it. Nothing is installed into a particular agent's skill directory.
- Prove macOS or Windows end to end. Those binaries are compiled only.
- Ship a license. Nothing in this tree grants one.

## Development

```sh
go test ./...
```

Cross builds, not run:

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/simplex-darwin-arm64 ./cmd/simplex
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/simplex-windows-amd64.exe ./cmd/simplex
```

`dist/` is gitignored.
