module github.com/Trojan295/discord-airplay

go 1.19

require (
	github.com/alphacep/vosk-api/go v0.3.45
	github.com/bwmarrin/discordgo v0.26.1
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0-20230115123313-8738427dd60b
	github.com/kelseyhightower/envconfig v1.4.0
	gopkg.in/hraban/opus.v2 v2.0.0-20220302220929-eeacdbcb92d0
)

require (
	github.com/gorilla/websocket v1.4.2 // indirect
	golang.org/x/crypto v0.0.0-20210421170649-83a5a9bb288b // indirect
	golang.org/x/sys v0.0.0-20201119102817-f84b799fce68 // indirect
)

replace github.com/ggerganov/whisper.cpp/bindings/go => ./whisper.cpp/bindings/go
