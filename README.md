# ConcertCloudMobilizonBot
A simple bot mirroring events from Concert Cloud to a Mobilizòn instance

## Installation

```bash
# clone the repo
git clone https://github.com/MarkJaroski/go-mobilizon-bot.git
# build
pushd go-mobilizon-bot && go build && popd
# install
pushd go-mobilizon-bot && go install && popd
```

Unless you have go configured to do something else the binary will be in ~/go/bin/

## Usage

```
Usage of ./go-mobilizon-bot:
      --actor string        The Mobilizon actor ID to use as the event organizer.
      --authconfig string   Use this file for authorization tokens. (default "/home/mark/.config/mobilizon/auth.json")
      --authorize           Authorize this bot and quit. An auth token and renew token will be output.
      --city string         The concertcloud API param 'city' (default "X")
      --config string       Use this directory for configuration. (default "/home/mark/.config/mobilizon")
      --country string      The concertcloud API param 'country'
      --date string         The concertcloud API param 'date'
      --debug               Debug mode.
      --draft               Create events in draft mode.
      --file string         Instead of fetching from concertcloud, use local file.
      --group string        The Mobilizon group ID to use for the event attribution.
      --limit string        The concertcloud API param 'limit'
      --noop                Gather all required information and report on it, but do not create events in Mobilizòn.
      --page string         The concertcloud API param 'page'
      --radius string       The concertcloud API param 'radius'
      --register            Register this bot and quit. A client id will be output.
      --timezone string     The timezone to use for the event attribution. (default "Europe/Zurich")
```

## Examples

TODO
