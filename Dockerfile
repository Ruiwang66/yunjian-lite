# Locked dependencies and explicit toolchain versions; runtime binaries are CGO-free.
FROM node:24.16.0-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.1-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY server/ server/
COPY agent/ agent/
COPY shared/ shared/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -tags nomsgpack -trimpath -ldflags="-s -w" -o /out/cloud-server ./server  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/downloads/cloud-agent-linux-amd64 ./agent  && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /out/downloads/cloud-agent-linux-arm64 ./agent  && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/downloads/cloud-agent.exe ./agent

FROM scratch
WORKDIR /app
COPY --from=go /out/cloud-server ./cloud-server
COPY --from=go /out/downloads ./downloads
COPY --from=web /src/web/dist ./web/dist
COPY --chown=10001:10001 deploy/data/ /app/data/
USER 10001:10001
ENV DATA_DIR=/app/data WEB_DIR=/app/web/dist LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/app/cloud-server", "--healthcheck"]
ENTRYPOINT ["/app/cloud-server"]
