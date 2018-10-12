# VSOP

VSOP is a fork of GIN that adds an interactive dashboard.

VSOP is currently a **work in progress**

## Introduction

`vsop` is a simple command line utility for live-reloading Go web applications.
Just run `vsop` in your app directory and your web app will be served with
`vsop` as a proxy. `vsop` will automatically recompile your code when it
detects a change. Your app will be restarted the next time it receives an
HTTP request.

## Features

- Monitor sub folders using [fsnotify](https://github.com/fsnotify/fsnotify)
- Log watcher
  - A regex find and filter
  - `tail -f` style auto scrolling
- `dep` support
- Desktop notification on Windows 10 (without or with bash see below) / Linux / OSX using [notificator](github.com/0xAX/notificator)

## Key Commands

### Global

| Keys | Command |
| --- | --- |
|`Ctrl+c`| quit |

### Log / Main View

| Keys      | Command |
| ---       | --- |
| `↵` (enter) | Insert new line (empty log message) |
| `↑` (up)  | Scroll up (mouse scroll also works, stops auto scroll) |
| `↓` (down)| Scroll down (mouse scroll also works, if you reach the end of the log it starts auto scroll) |
| `end`     | Go to end of log (will also start auto scroll |
| `ctrl+b`  | Build app |
| `ctrl+d`  | `dep ensure` |
| `ctrl+r`  | Run / restart app |
| `ctrl+k`  | Kill app |
| `tab`     | Toggle log group (all, app only, VSOP only) |
| `ctrl+f`  | Focus on find input |

## Find Input

| Keys      | Command |
| ---       | --- |
| `↵` (enter) | Return focus to log view |
| `tab`       | Toggle between find (highlight only) and filter (only show matching rows) |
| `ctrl+u`    | Clear input |

## Installation

Assuming you have a working Go environment and `GOPATH/bin` is in your
`PATH`, `vsop` is a breeze to install:

```shell
go get github.com/SamHennessy/vsop
```

Then verify that `vsop` was installed correctly:

```shell
vsop -h
```

## Basic usage

```shell
vsop run main.go
```

### Options

```shell
   --laddr value, -l value       listening address for the proxy server
   --port value, -p value        port for the proxy server (default: 3000)
   --appPort value, -a value     port for the Go web server (default: 3001)
   --bin value, -b value         name of generated binary file (default: "gin-bin")
   --path value, -t value        Path to watch files from (default: ".")
   --build value, -d value       Path to build files from (defaults to same value as --path)
   --excludeDir value, -x value  Relative directories to exclude
   --immediate, -i               run the server immediately after it's built
   --all                         reloads whenever any file changes, as opposed to reloading only on .go file change
   --godep, -g                   use godep when building
   --buildArgs value             Additional go build arguments
   --certFile value              TLS Certificate
   --keyFile value               TLS Certificate Key
   --logPrefix value             Setup custom log prefix
   --notifications               enable desktop notifications
   --help, -h                    show help
   --version, -v                 print the version
```

## Supporting VSOP in Your Web app

`vsop` assumes that your web app binds itself to the `PORT` environment
variable so it can properly proxy requests to your app. Web frameworks
like [Martini](http://github.com/codegangsta/martini) do this out of
the box.

## Using flags?

When you normally start your server with [flags](https://godoc.org/flag)
if you want to override any of them when running `vsop` it's suggest you
instead use [github.com/namsral/flag](https://github.com/namsral/flag)
as explained in [this post](http://stackoverflow.com/questions/24873883/organizing-environment-variables-golang/28160665#28160665)

## Known Issues

### Unable to delete folders

You may get an error when trying to delete a folder while VSOP is running. When this happens stop VSOP and delete the folder. This issue can cause a real mess when switching between branches in git or similar tool.

## Development

### Setup

`dep ensure -vendor-only`

### After you make a change

`go install`

## Notifications for Windows Subsystem for Linux  (WSL) / Bash on Windows

- Download notify send from http://vaskovsky.net/notify-send/
- unzip
- open a bash/linux terminal
- symlink the downloaded binary to a somewhere in you $PATH
- E.g. `sudo ln -s /mnt/c/Users/samhe/Downloads/notify-send/notify-send.exe /usr/local/bin/notify-send`