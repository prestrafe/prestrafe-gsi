## Building and running the GSI backend service locally

Start by checking out this repository and navigate into it within your terminal. The bot is shipped within a Docker
container that builds the executable and runs it directly afterwards. To build and run that container, perform the
following commands:

```powershell
docker build -t prestrafe-gsi:dev .
docker run --rm --name prestrafe-gsi -p 8080:8080 -it prestrafe-gsi:dev
```

To make your CSGO game send actual data to the GSI backend, you need create the following config file:

```
"Prestrafe Bot Game Integration Configuration (Development)"
{
    "uri" "http://localhost:8080/update"
    "timeout" "1.0"
    "buffer" "0.1"
    "throttle" "2.5"
    "heartbeat" "2.5"
    "auth"
    {
        // Replace the xxx with the GSI token you want to use inside the config.yml for the prestrafe-bot
        "token" "xxx"
    }
    "data"
    {
        "provider" "1"
        "map" "1"
        "round" "1"
        "player_id" "1"
        "player_state" "1"
        "player_match_stats" "1"
    }
}
```

## Deployment Trigger

Number: 1
