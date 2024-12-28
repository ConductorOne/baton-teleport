# On-Premises Baton-Teleport
for more details on any of the steps follow this guide:  
https://goteleport.com/docs/enroll-resources/machine-id/deployment/linux/

## Requirements
- **Install Teleport**
```bash
TELEPORT_EDITION="enterprise"
TELEPORT_VERSION="16.4.12"
curl https://cdn.teleport.dev/install-v16.4.12.sh | bash -s ${TELEPORT_VERSION?} ${TELEPORT_EDITION?}
```


## 1. Create a bot
```bash
tctl bots add example
```

## 2. Create a /etc/tbot.yaml file
- replace `<your example.teleport.sh:443 >` with your teleport server address
- replace `<your token>` with the token you got from the bot creation

```yaml
version: v2
proxy_server: <your example.teleport.sh:443 >
onboarding:
  join_method: token
  token: <your token>
storage:
  type: directory
  path: /var/lib/teleport/bot
outputs: 
  - type: identity
    destination:
      type: directory
      path: /opt/machine-id
```

## 3. Add roles to the bot
https://goteleport.com/docs/reference/access-controls/roles#preset-roles

```bash
tctl bots update example --add-roles "access,auditor,editor"
```

## 4. Run the tbot
```bash
tbot -c /etc/tbot.yaml start
```
# Run as a service
https://goteleport.com/docs/enroll-resources/machine-id/deployment/linux/#create-a-systemd-service
