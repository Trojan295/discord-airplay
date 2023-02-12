FROM golang:1.19 AS builder

RUN apt-get update \
  && apt-get install -y build-essential libopus-dev libopusfile-dev \
  && go install github.com/bwmarrin/dca/cmd/dca@latest

WORKDIR /src/

COPY go.mod go.sum .
COPY whisper.cpp whisper.cpp
RUN go mod download

COPY . .

RUN cd whisper.cpp && \
  make libwhisper.a && \
  ./models/download-ggml-model.sh base.en && \
  mv ./models/ggml-base.en.bin ./models/model.bin

RUN CGO_ENABLED=1 GOARCH=amd64 CGO_CFLAGS=-I$(pwd)/whisper.cpp LIBRARY_PATH=$(pwd)/whisper.cpp go build -tags whisper -o /bin/airplay cmd/airplay/airplay.go

FROM ubuntu

RUN apt-get update \
  && apt-get install -y ffmpeg wget libopusfile0 \
  && wget https://github.com/yt-dlp/yt-dlp/releases/download/2022.11.11/yt-dlp_linux -O /usr/local/bin/yt-dlp \
  && chmod +x /usr/local/bin/yt-dlp

COPY --from=builder /src/whisper.cpp/models/model.bin /bin/model.bin
COPY --from=builder /bin/airplay /bin/airplay
COPY --from=builder /go/bin/dca /usr/local/bin/dca

CMD ["/bin/airplay"]
