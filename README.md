# `baton-teleport` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-teleport.svg)](https://pkg.go.dev/github.com/conductorone/baton-teleport) ![main ci](https://github.com/conductorone/baton-teleport/actions/workflows/main.yaml/badge.svg)
`baton-teleport` is a connector for teleport built using the [Baton SDK](https://github.com/conductorone/baton-sdk). It communicates with the teleport API to sync data about users, groups and enterprise.

Check out [Baton](https://github.com/conductorone/baton) to learn more the project in general.

# Getting Started
You can try out the teleport platform with a free, 14-day trial account. If you're interested in becoming a teleport developer partner, you can convert your trial account into a sponsored teleport Support account.

As part of becoming a teleport developer partner, teleport sponsors an instance for up to 5 agents that you can use for developing, and troubleshooting your app or integration.

Unlike a trial account, a sponsored account does not expire after 14 days.
## Prerequisites

1. Teleport `trial account` sign up for a free teleport Support trial  [developer site](https://goteleport.com/signup/)
2. Application Scopes: 
  - users
  - roles
  - apps
  - databases
  - grant resources
  - revoke resources

## brew

```
brew install conductorone/baton/baton conductorone/baton/baton-teleport
baton-teleport
baton resources
```

## docker

```
docker run --rm -v $(pwd):/out -e BATON_PROXYADDR=clientProxy ghcr.io/conductorone/baton-teleport:latest -f "/out/sync.c1z"
docker run --rm -v $(pwd):/out ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources
```

## source

```
go install github.com/conductorone/baton/cmd/baton@main
go install github.com/conductorone/baton-teleport/cmd/baton-teleport@main

BATON_PROXYADDR=clientProxy baton-teleport 
baton resources
```

# Data Model

`baton-teleport` pulls down information about the following teleport resources:
- Users
- Roles
- Apps
- Databases

# Running a teleport instance

### Replace `<email_account>` and `<cluster_name>` with your cluster credentials
```
1. Add teleport yaml file
sudo teleport configure -o file \
--acme --acme-email=<email_account> \
--cluster-name=<cluster_name>

2. Start teleport using our previous yaml file
sudo teleport start --config="/etc/teleport.yaml"

3. Logging your teleport cluster
tsh login --proxy=<cluster_name> --user=<email_account>

4. Generating auth.pem file using tctl
TELEPORT_CONFIG_FILE="" tctl auth sign --ttl=8h --user=<email_account> --out=auth.pem
```

# Contributing, Support, and Issues

We started Baton because we were tired of taking screenshots and manually building spreadsheets. We welcome contributions, and ideas, no matter how small -- our goal is to make identity and permissions sprawl less painful for everyone. If you have questions, concerns, or ideas: Please open a Github Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.

# `baton-teleport` Command Line Usage

```
baton-teleport

Usage:
  baton-teleport [flags]
  baton-teleport [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  help               Help about any command

Flags:
      --client-id string       The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string   The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
  -f, --file string            The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                   help for baton-teleport
      --log-format string      The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string       The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -p, --provisioning           This must be set in order for provisioning actions to be enabled. ($BATON_PROVISIONING)
      --proxyAddr string       The fully-qualified teleport proxy service to connect with. Example: "baton.teleport.sh:443" ($BATON_PROXYADDR)
  -v, --version                version for baton-teleport

Use "baton-teleport [command] --help" for more information about a command.
```
