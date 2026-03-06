# Teleport Setup Guide for baton-teleport

This guide walks you through everything needed to get `baton-teleport` running locally for development and testing.

## Prerequisites

- **Go** 1.23+ installed
- **macOS**, Linux, or Windows
- A Teleport cluster (cloud trial or self-hosted)

## Step 1: Get a Teleport Cluster

The fastest option is a free Teleport Cloud trial:

1. Sign up at [goteleport.com/signup](https://goteleport.com/signup/)
2. Follow the onboarding wizard to create your cluster (e.g., `yourname.teleport.sh`)
3. Create your admin user and set up MFA (you'll need an authenticator app like Google Authenticator, Authy, or 1Password)

## Step 2: Install Teleport CLI Tools

You need `tsh` (client) and `tctl` (admin) CLI tools.

### macOS

Download and install the `.pkg`:

```bash
curl -O https://cdn.teleport.dev/teleport-18.2.2.pkg
```

Then double-click the `.pkg` file in Finder to install, or run:

```bash
sudo installer -pkg teleport-18.2.2.pkg -target /
```

### Linux

```bash
curl -O https://cdn.teleport.dev/teleport-v18.2.2-linux-amd64-bin.tar.gz
tar -xzf teleport-v18.2.2-linux-amd64-bin.tar.gz
cd teleport
sudo ./install
```

### Verify Installation

Open a **new terminal** and run:

```bash
tsh version
```

You should see output like:

```
Teleport v18.2.2 git:v18.2.2-0-gb1ba1d1 go1.24.7
```

## Step 3: Log In to Your Cluster

```bash
tsh login --proxy=<cluster_name>.teleport.sh --user=<your_email>
```

This will prompt for:
1. **Password** - your Teleport account password
2. **OTP code** - the 6-digit code from your authenticator app

Example:

```bash
tsh login --proxy=mycompany.teleport.sh --user=admin@example.com
```

On success you'll see:

```
> Profile URL:        https://mycompany.teleport.sh:443
  Logged in as:       admin@example.com
  Cluster:            mycompany.teleport.sh
  Roles:              access, auditor, editor
  Valid until:        2026-02-13 05:47:26 [valid for 12h0m0s]
```

## Step 4: Generate the Identity File (auth.pem)

The `auth.pem` file is what the connector uses to authenticate with Teleport.

```bash
tctl auth sign --ttl=8h --user=<your_email> --out=auth.pem
```

Example:

```bash
tctl auth sign --ttl=8h --user=admin@example.com --out=auth.pem
```

> **Note:** This file expires after the TTL you set (8 hours in this example). You'll need to regenerate it after it expires by running the same command again (you may need to `tsh login` again first).

## Step 5: Build the Connector

```bash
make build
```

The binary will be at `dist/<os>_<arch>/baton-teleport`.

## Step 6: Run a Sync

```bash
./dist/darwin_arm64/baton-teleport \
  --teleport-proxy-address=<cluster_name>.teleport.sh:443 \
  --teleport-key-path=auth.pem \
  --log-level=debug \
  --log-format=console
```

Or using environment variables:

```bash
export BATON_TELEPORT_PROXY_ADDRESS=<cluster_name>.teleport.sh:443
export BATON_TELEPORT_KEY_PATH=auth.pem
./dist/darwin_arm64/baton-teleport --log-level=debug --log-format=console
```

## Step 7: Inspect the Results

```bash
# View all synced resources
baton resources -f sync.c1z

# View sync statistics
baton stats -f sync.c1z

# View grants (who has what access)
baton grants -f sync.c1z
```

Expected output:

```
Type           | Count
app            | 0
database       | 0
entitlements   | 18
grants         | 4
node           | 0
resource_types | 5
role           | 18
user           | 2
```

## Step 8: Test Provisioning (Optional)

To test granting and revoking roles:

```bash
# Run with provisioning enabled
./dist/darwin_arm64/baton-teleport \
  --teleport-proxy-address=<cluster_name>.teleport.sh:443 \
  --teleport-key-path=auth.pem \
  --provisioning

# Grant a role to a user
baton grant \
  --entitlement "role:<role-name>:member" \
  --principal-type user \
  --principal "<username>"

# Re-sync to see the change
./dist/darwin_arm64/baton-teleport \
  --teleport-proxy-address=<cluster_name>.teleport.sh:443 \
  --teleport-key-path=auth.pem

# Check grants
baton grants -f sync.c1z

# Revoke the grant
baton revoke --grant "<grant-id>"
```

## Troubleshooting

### `zsh: command not found: tsh`

The Teleport CLI tools aren't installed or not in your PATH. Install them (Step 2) and open a new terminal.

### `auth.pem` expired

Regenerate it:

```bash
tsh login --proxy=<cluster_name>.teleport.sh --user=<your_email>
tctl auth sign --ttl=8h --user=<your_email> --out=auth.pem
```

### MFA / OTP code required

You need an authenticator app (Google Authenticator, Authy, etc.) with your Teleport account enrolled. If you haven't set up MFA, log in via the web UI at `https://<cluster_name>.teleport.sh` first -- it will walk you through MFA enrollment.

### Connection timeout

- Make sure you include the port: `<cluster_name>.teleport.sh:443`
- Check that your network allows outbound connections to port 443

### `*.pem` files in `.gitignore`

The `auth.pem` file is already excluded from git via `.gitignore`. Never commit identity files to version control.

## Configuration Reference

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--teleport-proxy-address` | `BATON_TELEPORT_PROXY_ADDRESS` | Teleport proxy address (e.g., `myco.teleport.sh:443`) |
| `--teleport-key-path` | `BATON_TELEPORT_KEY_PATH` | Path to identity file (e.g., `auth.pem`) |
| `--teleport-key` | `BATON_TELEPORT_KEY` | Identity file contents as a string (alternative to key-path) |
| `--provisioning` | `BATON_PROVISIONING` | Enable provisioning (grant/revoke) |
| `--log-level` | `BATON_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-format` | `BATON_LOG_FORMAT` | Log format: `json`, `console` |
