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
      --actor string          The Mobilizon actor ID to use as the event organizer.
      --authconfig string     Use this file for authorization tokens. (default "/home/mark/.config/mobilizon/auth.json")
      --authorize             Authorize this bot and quit. An auth token and renew token will be output.
      --city string           The concertcloud API param 'city' (default "X")
      --config string         Use this directory for configuration. (default "/home/mark/.config/mobilizon")
      --country string        The concertcloud API param 'country'
      --date string           The concertcloud API param 'date'
      --debug                 Debug mode.
      --draft                 Create events in draft mode.
      --file string           Instead of fetching from concertcloud, use local file.
      --group string          The Mobilizon group ID to use for the event attribution.
      --limit string          The concertcloud API param 'limit'
      --mobilizonurl string   Your Mobilizon base URL (default "https://mobilisons.ch")
      --noop                  Gather all required information and report on it, but do not create events in Mobilizòn.
      --page string           The concertcloud API param 'page'
      --radius string         The concertcloud API param 'radius'
      --register              Register this bot and quit. A client id will be output.
      --timezone string       The timezone to use for the event attribution. (default "Europe/Zurich")
```
## Setup

Once you've built the bot you'll need to register it. 

```bash

./go/bin/go-mobilizon-bot --mobilizonurl <your-mobilizon-instance> --register

```

This will output a line which you can run in bash or zsh to set up your
environment for the next step: authorization.

```bash

export GRAPHQL_CLIENT_ID=<your-id>

./go/bin/go-mobilizon-bot --mobilizonurl <your-mobilizon-instance> --authorize

```

Unless there is an HTTP error this should result in the device code
handshake, which should be familiar to anybody who has set up a streaming
service on a "smart" TV:

```
Please visit this URL and enter the code below https://mobilisons.ch/login/device

XXXX-XXXX

Then press any key to continue.
```

![image](https://github.com/user-attachments/assets/0d18d89d-1306-4d95-953b-b0b7df8379d1)

You can check the results on the bot server at 

`~/.config/mobilizon/auth.json`

and on your Mobilizon instance at the path:

`/settings/authorized-apps`


## Examples

First, you'll need to obtain the actorid and groupid you want to post as.
So far the best way to do this is using your browser's developer tools, and
grabbing the values from a GraphQL query.

See #8

Then, if your goal is to upload events from ConcertCloud you just need a
city name, and a download limit, unless you are ready for the whole events
list

```
./go/bin/go-mobilizon-bot --city=Lausanne --actor=<actorid> --group=<groupid> --limit=1024
```

Or a country name:

```
/go-mobilizon-bot --country=Switzerland --actor=<actorid> --group=<groupid> --limit=2000
```


Or if you prefer generate a local goskyr config and upload from the
resulting events json you can something like this:

```
./go-mobilizon-bot --file goskyr-config/json/polesud.json --actor=<actorid> --group=<groupid>
```

There are systemd unit files in the `/examples` directory which should help
you set up your mobilizon upload job.



