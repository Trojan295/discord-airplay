FROM golang:1.19 AS builder

RUN apt-get update \
  && apt-get install -y build-essential \
  && go install github.com/bwmarrin/dca/cmd/dca@latest

WORKDIR /src/

COPY go.mod go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOARCH=amd64 go build -ldflags "-s -w" -o /bin/airplay cmd/airplay/airplay.go

FROM ubuntu

RUN apt-get update \
  && apt-get install -y ffmpeg wget \
  && wget https://github.com/yt-dlp/yt-dlp/releases/download/2022.11.11/yt-dlp_linux -O /usr/local/bin/yt-dlp \
  && chmod +x /usr/local/bin/yt-dlp

COPY --from=builder /bin/airplay /bin/airplay
COPY --from=builder /go/bin/dca /usr/local/bin/dca

CMD ["/bin/airplay"]
