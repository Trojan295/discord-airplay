# discord-airplay

Discord bot to play music

## Features

- Playing songs from various sources (Big thanks to [yt-dlp](https://github.com/yt-dlp/yt-dlp)!)
- Playing playlists from Youtube
- Playlist generation using ChatGPT

## How to use it

There is no public bot available, you have to host it on your own.

1. Create the Discord bot and get the token. You can do this [here](https://discord.com/developers/applications).
2. If you want to use the ChatGPT playlist generation get an OpenAI account and API token.
3. Run the bot.

    You can run it using a Docker container:
    ```bash
    docker run -it \
      -e AIR_DISCORDTOKEN="<discord-token>" \
      -e AIR_OPENAITOKEN="<openai-token>" \
      trojan295/airplay
    ```

    You can also run it on Kubernetes:
    ```bash
    cat deploy/kubernetes/deployment.yaml \
      | env AIR_DISCORDTOKEN="<discord-token>" \
        AIR_OPENAITOKEN="<openai-token>" \
        envsubst \
      | kubectl apply -f -
    ```
