# Ollama Discord Bot in Go
Utilize the Ollama api in your discord server

Tagging the bot in a message will prompt ollama with that message

**Discord Bot Necessities**
- **Scopes**
    * bot
    * applications.commands
- **Bot Permissions**
    * Send Messages
    * Read Message History
- **Privileged Gateway Intents**
    * Message Content Intent
## Discord Commands
```
/current
    returns current selected model
/delete <num>
    deletes previous bot messages
/list
    lists downloaded models
/llm <model-name>
    changes current selected model
/manipulate <msg>
    sends message as assistant to influence future messages
/timeout <string>
    changes the time it takes for the LLM to be loaded out of memory
```
## Docker
[![Static Badge](https://img.shields.io/badge/Docker-161B22?style=for-the-badge&logo=docker)](https://hub.docker.com/repository/docker/ethanscully/ollama-discord/)

```Shell
docker run --name ollama-discord \
    --restart unless-stopped \
    -e TOKEN="Discord-Bot-Token" \
    -e HOSTNAME="URL-to-Ollama-Api" \
    -e MODEL="LLM-Model" \
    -e KEEPLOADED="Time-to-Unload-From-Memory" \
    -itd ethanscully/ollama-discord
```
example:
```Shell
docker run --name ollama-discord \
    --restart unless-stopped \
    -e TOKEN="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
    -e HOSTNAME="http://127.0.0.1:11434/" \
    -e MODEL="llama3:8b" \
    -e KEEPLOADED="30m" \
    -itd ethanscully/ollama-discord
```
### Environment Variables
|Variables|Description|
|-|-|
|**TOKEN**|Token of the Discord Bot|
|**HOSTNAME**|URL and port of the Ollama API server ex: `http://127.0.0.1:11434/`|
|**MODEL**|Default LLM Model to be used|
|**KEEPLOADED**|Default Time for LLM Model to be unloaded from memory in minutes|
|**SYSPROMPT**|Custom System Prompt
## Build
```Shell
go build
```
### Docker
```Shell
docker build -t ollama-discord .
```
### Notes

System prompt - buggy, ollama bug
- Custom System Prompt causes unpredictable or empty response from the ollama api

Multimodal image vision disabled due to ollama bug
- After identifing an image, model can't see future images unless LLM is unloaded from memory
