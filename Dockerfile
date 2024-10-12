FROM golang:1.23 AS builder

WORKDIR /src/
RUN apt-get update && apt-get install -y libopus-dev libopusfile-dev

COPY go.mod go.sum .
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOARCH=amd64 go build -ldflags '-w -s' -o /bin/airplay cmd/airplay/airplay.go

FROM ubuntu:24.10

ARG YT_DLP_VERSION="2024.10.07"

RUN apt-get update \
  && apt-get install -y ffmpeg wget libopusfile0 \
  && wget "https://github.com/yt-dlp/yt-dlp/releases/download/${YT_DLP_VERSION}/yt-dlp_linux" -O /usr/local/bin/yt-dlp \
  && chmod +x /usr/local/bin/yt-dlp

COPY --from=builder /bin/airplay /bin/airplay

CMD ["/bin/airplay"]
