---
name: simplex
description: Use when sending or receiving SimpleX messages, files, or pictures with the simplex CLI or the Herdr plugin simplex.agents. Covers text, file rows versus inline pictures, groups, inbox, ack, and MCP.
---

# simplex

One profile is one SimpleX identity, one local daemon, and one inbox. These commands are the whole interface. `simplex` and `simplex help` print this text. `simplex plugin status` prints the daemon status and then this text.

`--profile NAME` selects a profile. `SIMPLEX_PROFILE` does the same. `--json` prints the raw result.

## Paths

| | |
| --- | --- |
| Linux and macOS profile | `~/.config/simplex/profiles/<name>/` |
| Windows profile | `%APPDATA%\simplex\profiles\<name>\` which is `C:\Users\<you>\AppData\Roaming\simplex\profiles\<name>` |
| Override | `SIMPLEX_HOME` |
| CLI | `~/.local/bin/simplex` or `%USERPROFILE%\.local\bin\simplex.exe` |
| Official chat program | `~/.local/share/simplex/bin/simplex-chat` (`.exe` on Windows). Override with `SIMPLEX_CHAT_BIN` |
| Received files | `<profile>/files/` |

## Setup

Windows, without Go:

```powershell
irm https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.ps1 | iex
```

`herdr plugin install` on Windows runs `build.ps1` and needs Go. Use the PowerShell line unless Go is installed.

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/arcticfoxweb/simplex-herdr/main/install.sh | sh
```

From the Herdr pane where this agent is already running:

```sh
simplex init
simplex qr
herdr plugin action invoke simplex.agents.attach
```

`simplex init [name] --pane PANE` creates the profile and prints its address. `simplex qr` draws a small square QR of the short `https://` link. The address is a capability: anyone who has it can message that agent. `simplex address` prints the long address. `simplex address --json` includes the short link.

Another agent connects with:

```sh
simplex --profile OTHER connect "<address>"
```

`simplex use NAME` selects the default profile. `simplex profiles` lists them. `simplex up` runs the daemon in the foreground. `simplex down` stops it. `simplex status` shows the daemon, address, unread count, and pane.

On Windows, `simplex-chat.exe` needs `libcrypto-3-x64.dll` beside it. The installer adds that DLL and, if the program still exits `0xc0000135`, `libssl-3-x64.dll`. Do not treat a chat binary that exits `0xc0000135` as installed.

## Send text

```sh
simplex send NAME "hello"
simplex send NAME -
```

`simplex send NAME -` reads the message from stdin.

Through the plugin, actions take no extra arguments. The recipient and the text are environment variables. This pane sends text only:

```sh
herdr plugin pane open --plugin simplex.agents --entrypoint send \
  --env SIMPLEX_TO=NAME \
  --env SIMPLEX_TEXT='hello'
```

The pane closes after a successful send. `SIMPLEX_TO` is a contact name or a group name.

## Files and pictures

```sh
simplex send-file NAME ./notes.txt
simplex send-file NAME ./notes.txt "caption"
```

`send-file` always sends `msgContent.type` `text` plus a `fileSource`. The SimpleX app shows that as a file row named like `1.jpg` or `monkey.jpg`. The app draws an inline picture only when `msgContent.type` is `image` and the message includes an image preview.

This program does not build an image message. `send-file` and the Herdr plugin cannot make the app show a photo. A picture that appears inline was sent through the SimpleX chat API directly. Use `send-file` when the other side should receive the file.

Incoming files are accepted automatically, up to 100MB, into the profile `files` directory. The inbox line includes `file:` only when a real local path exists.

There is no plugin pane for files. Use `simplex send-file`.

## Groups

A group you have already joined is a name:

```sh
simplex send "Agents" "hello"
simplex send "#Agents" "hello"
simplex send-file "#Agents" ./notes.txt
```

A leading `#` forces a group when a contact uses the same name. There is no command to create a group, invite members, or join one. Accept the invitation in SimpleX Chat, then send by the group name.

## Receive

The daemon submits the oldest unread message into the attached Herdr pane only when that agent is idle or done and the screen has stayed still. It does not type over output or over a line still being written. The default is the next stable idle look.

```sh
simplex gateway PANE
simplex gateway PANE --quiet 2s
simplex gateway off
```

`--quiet 2s` waits that long after the screen settles. The minimum is 200ms.

A direct line looks like `NAME says [msg:direct:<contactId>:<itemId>]: text`. A group line looks like `<speaker> says in <group> [msg:group:<groupId>:<itemId>]: text`.

```sh
simplex inbox
simplex inbox --wait 30s
simplex inbox --all
simplex ack msg:direct:2:10
```

`inbox` lists what is still unread. `--all` includes messages already delivered or acked. `--wait` blocks until something is unread. After you have dealt with a message, `ack` its id. Reply once.

Incoming calls are rejected and are not typed into the pane. Join notices, encryption banners, and group setting events are not messages.

## Herdr plugin

Plugin id `simplex.agents`.

```sh
herdr plugin action invoke simplex.agents.attach
herdr plugin action invoke simplex.agents.status
herdr plugin action invoke simplex.agents.start
```

`attach` points the current pane at this profile and starts the daemon. `status` prints daemon status and these instructions. `start` and the startup hook ensure the default profile daemon is up. Chat text is submitted into the pane. It does not appear in the plugin log.

## MCP

Any agent that can launch a stdio server, one process per profile:

```toml
command = "simplex"
args = ["mcp", "--profile", "default"]
```

Tools: `address`, `connect`, `contacts`, `send`, `send_file`, `inbox`, `ack`.

`send_file` is the same text-plus-file message as `simplex send-file`. It does not send an image preview. Prefix a group with `#` when it shares a name with a contact.

## Not available

- An image message the SimpleX app shows as a picture.
- Creating a group, inviting members, or joining from the CLI.
- Voice or video. Incoming calls are rejected.
- A license. Nothing in this tree grants one.
