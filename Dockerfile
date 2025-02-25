FROM golang:1.19.0-alpine3.16 AS build
WORKDIR /srv
COPY src/ .
COPY config.json config.json
RUN apk update && \
    apk add --no-cache gcc musl-dev && \
    rm -rf /var/lib/apt/lists/*
RUN go run ./utils/defaultUserRegister/.
RUN go run ./utils/defaultGroupAccessRegister/.
RUN go build -o ./bin/ipehr-gateway cmd/ipehrgw/main.go

FROM alpine:3.16
WORKDIR /srv
COPY data/ /data
COPY --from=build /srv/bin/ /srv
COPY --from=build /srv/config.json /srv
CMD ["./ipehr-gateway", "-config=./config.json"]
