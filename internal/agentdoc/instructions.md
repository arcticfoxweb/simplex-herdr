---
name: simplex
description: Use when sending or receiving SimpleX messages, files, or pictures with the simplex CLI, the Herdr plugin simplex.agents, or the local simplex-chat WebSocket API. Covers the full chat command surface, including image messages and groups.
---

# simplex

One profile is one SimpleX identity, one local daemon, and one inbox. The `simplex` commands below are wrappers. The daemon's `simplex-chat` process is the API. Anything the wrappers do not build, send on that API. `simplex` and `simplex help` print this text. `simplex plugin status` prints the daemon status and then this text.

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

The chat API port is `port` in `<profile>/profile.json`. The socket is `ws://127.0.0.1:<port>`. `userId` in that file is the user id for commands that take one. It is usually `1`.

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

## CLI wrappers

```sh
simplex send NAME "hello"
simplex send NAME -
simplex send-file NAME ./notes.txt
simplex send-file NAME ./notes.txt "caption"
simplex send "#Group" "hello"
```

`simplex send NAME -` reads the message from stdin. A leading `#` forces a group when a contact uses the same name.

`send-file` always sends `msgContent.type` `text` plus a `fileSource`. The SimpleX app shows that as a file row named like `1.jpg`. It does not draw an inline picture. Use the chat API image message below for a picture.

Through the plugin, actions take no extra arguments. The send pane is text only:

```sh
herdr plugin pane open --plugin simplex.agents --entrypoint send \
  --env SIMPLEX_TO=NAME \
  --env SIMPLEX_TEXT='hello'
```

The pane closes after a successful send. There is no plugin pane for files. Use `simplex send-file` or the API.

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

Incoming files are accepted automatically, up to 100MB, into the profile `files` directory. The inbox line includes `file:` only when a real local path exists.

This daemon rejects incoming calls and does not type them into the pane. Join notices, encryption banners, and group setting events are not submitted as messages.

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

`send_file` is the same text-plus-file message as `simplex send-file`. It does not send an image preview. Prefix a group with `#` when it shares a name with a contact. For an image, a group join, or any command this tool list does not wrap, use the chat API.

## Chat API

Connect a second WebSocket to `ws://127.0.0.1:<port>` from the profile. Leave the daemon running. Each command is one JSON text frame:

```json
{"corrId":"1","cmd":"/_send @3 json [...]"}
```

`corrId` is any unique string. The matching response has the same `corrId` and a `resp` object. Frames with an empty `corrId` are events. A failed command comes back as `resp.type` `chatCmdError`.

`@<contactId>` is a contact. `#<groupId>` is a group. Those ids are the numbers in inbox ids `msg:direct:<contactId>:<itemId>` and `msg:group:<groupId>:<itemId>`.

`/_send` takes a JSON array of composed messages. Each object has `msgContent`, optional `fileSource`, optional `quotedItemId`, and `mentions` (use `{}` when there are none). `fileSource` is `{"filePath":"/absolute/path"}`.

`msgContent.type` is one of:

| type | fields | what the app shows |
| --- | --- | --- |
| `text` | `text` | a text bubble |
| `link` | `text`, `preview` | a link with a preview |
| `image` | `text`, `image` | an inline picture when `image` is the preview and `fileSource` is the file |
| `video` | `text`, `image`, `duration` | a video |
| `voice` | `text`, `duration` | a voice message |
| `file` | `text` | a file row |
| `chat` | `text`, `chatLink` | a chat link |
| `report` | `text`, `reason` | a report |

A picture the app draws is `type` `image`, not `text` with a jpeg attached. `image` is the preview string. `fileSource.filePath` is the file to upload. `text` is the caption and may be empty.

```json
{"corrId":"1","cmd":"/_send @3 json [{\"msgContent\":{\"type\":\"image\",\"text\":\"\",\"image\":\"<preview>\"},\"fileSource\":{\"filePath\":\"C:\\\\Users\\\\me\\\\photo.jpg\"},\"mentions\":{}}]"}
```

This is the stable WebSocket command surface from [COMMANDS.md](https://github.com/simplex-chat/simplex-chat/blob/stable/bots/api/COMMANDS.md). Brackets mark optional flags. `json(...)` is one JSON value. Field records for those JSON values are in [TYPES.md](https://github.com/simplex-chat/simplex-chat/blob/stable/bots/api/TYPES.md). `sendRef` and `chatRef` are `@<contactId>` or `#<groupId>`.

```text
APICreateMyAddress              /_address <userId>
APIDeleteMyAddress              /_delete_address <userId>
APIShowMyAddress                /_show_address <userId>
APISetProfileAddress            /_profile_address <userId> on|off
APISetAddressSettings           /_address_settings <userId> <json(settings)>

APISendMessages                 /_send <str(sendRef)>[ live=on][ ttl=<ttl>][ sign=on] json <json(composedMessages)>
APIUpdateChatItem               /_update item <str(chatRef)> <chatItemId>[ live=on] json <json(updatedMessage)>
APIDeleteChatItem               /_delete item <str(chatRef)> <chatItemIds[0]>[,<chatItemIds[1]>...] broadcast|internal|internalMark|history
APIDeleteMemberChatItem         /_delete member item #<groupId> <chatItemIds[0]>[,<chatItemIds[1]>...]
APIChatItemReaction             /_reaction <str(chatRef)> <chatItemId> on|off <json(reaction)>

ReceiveFile                     /freceive <fileId>[ approved_relays=on][ encrypt=on|off][ inline=on|off][ <filePath>]
CancelFile                      /fcancel <fileId>

APINewGroup                     /_group <userId>[ incognito=on] <json(groupProfile)>
APINewPublicGroup               /_public group <userId>[ incognito=on] <relayIds[0]>[,<relayIds[1]>...] <json(groupProfile)>
APIListGroups                   /_groups <userId>[ @<contactId_>][ <search>]
APIJoinGroup                    /_join #<groupId>
APILeaveGroup                   /_leave #<groupId>
APIListMembers                  /_members #<groupId>
APIAddMember                    /_add #<groupId> <contactId> relay|observer|author|member|moderator|admin|owner
APIAcceptMember                 /_accept member #<groupId> <groupMemberId> relay|observer|author|member|moderator|admin|owner
APIMembersRole                  /_member role #<groupId> <groupMemberIds[0]>[,<groupMemberIds[1]>...] relay|observer|author|member|moderator|admin|owner
APIBlockMembersForAll           /_block #<groupId> <groupMemberIds[0]>[,<groupMemberIds[1]>...] blocked=on|off
APIRemoveMembers                /_remove #<groupId> <groupMemberIds[0]>[,<groupMemberIds[1]>...][ messages=on]
APIUpdateGroupProfile           /_group_profile #<groupId> <json(groupProfile)>
APIGetGroupRelays               /_get relays #<groupId>
APIAddGroupRelays               /_add relays #<groupId> <relayIds[0]>[,<relayIds[1]>...]
APIAllowRelayGroup              /_relay allow #<groupId>

APICreateGroupLink              /_create link #<groupId> relay|observer|author|member|moderator|admin|owner
APIGroupLinkMemberRole          /_set link role #<groupId> relay|observer|author|member|moderator|admin|owner
APIDeleteGroupLink              /_delete link #<groupId>
APIGetGroupLink                 /_get link #<groupId>

APIAddContact                   /_connect <userId>[ incognito=on]
APIConnectPlan                  /_connect plan <userId> <connectTarget>
APIConnect                      /_connect <userId>[ <str(preparedLink_)>]
Connect                         /connect[ <connTarget_>]
APIAcceptContact                /_accept <contactReqId>
APIRejectContact                /_reject <contactReqId>
APIListContacts                 /_contacts <userId>

APIGetChats                     /_get chats <userId>[ pcc=on] <str(pagination)> <json(query)>
APIDeleteChat                   /_delete <str(chatRef)> <str(chatDeleteMode)>
APISetContactCustomData         /_set custom @<contactId>[ <json(customData)>]
APISetGroupCustomData           /_set custom #<groupId>[ <json(customData)>]
APISetUserAutoAcceptMemberContacts  /_set accept member contacts <userId> on|off
APISetContactPrefs              /_set prefs @<contactId> <json(preferences)>

ShowActiveUser                  /user
ListUsers                       /users
APISetActiveUser                /_user <userId>[ <json(viewPwd)>]
CreateActiveUser                /_create user <json(newUser)>
APIDeleteUser                   /_delete user <userId> del_smp=on|off[ <json(viewPwd)>]
APIUpdateProfile                /_profile <userId> <json(profile)>
StartChat                       /_start
APIStopChat                     /_stop
```

Events arrive with an empty `corrId`. Their `resp.type` is one of the names in [EVENTS.md](https://github.com/simplex-chat/simplex-chat/blob/stable/bots/api/EVENTS.md):

```text
ContactConnected
ContactUpdated
ContactDeletedByContact
ReceivedContactRequest
NewMemberContactReceivedInv
ContactSndReady
NewChatItems
ChatItemReaction
ChatItemsDeleted
ChatItemUpdated
GroupChatItemsDeleted
ChatItemsStatusesUpdated
ReceivedGroupInvitation
UserJoinedGroup
GroupUpdated
JoinedGroupMember
MemberRole
DeletedMember
LeftMember
DeletedMemberUser
GroupDeleted
ConnectedToGroupMember
MemberAcceptedByOther
MemberBlockedForAll
GroupMemberUpdated
GroupLinkDataUpdated
GroupRelayUpdated
RcvFileDescrReady
RcvFileComplete
SndFileCompleteXFTP
RcvFileStart
RcvFileSndCancelled
RcvFileAccepted
RcvFileError
RcvFileWarning
SndFileError
SndFileWarning
AcceptingContactRequest
AcceptingBusinessRequest
ContactConnecting
BusinessLinkConnecting
JoinedGroupMemberConnecting
SentGroupInvitation
GroupLinkConnecting
HostConnected
HostDisconnected
SubscriptionStatus
MessageError
ChatError
ChatErrors
```

A group profile JSON needs `displayName` and `fullName`. Member roles are `relay`, `observer`, `author`, `member`, `moderator`, `admin`, and `owner`.

There is no license file in this repository. Nothing here grants one.
