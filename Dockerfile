# syntax=docker/dockerfile:1
FROM alpine
WORKDIR /local
COPY build/vipcast-linux-amd64-dev /local/
ENTRYPOINT ["./vipcast-linux-amd64-dev"]
