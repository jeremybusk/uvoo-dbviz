# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY web ./
RUN npm run build

FROM golang:1.22-alpine AS go
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/uvoo-sqvizerver ./cmd/uvoo-sqvizerver

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 uvoo-sqviz
WORKDIR /app
COPY --from=go /out/uvoo-sqvizerver /app/uvoo-sqvizerver
COPY --from=web /src/web/dist /app/web/dist
USER uvoo-sqviz
ENV SQVIZ_ADDR=:8080 SQVIZ_WEB_ROOT=/app/web/dist
EXPOSE 8080
ENTRYPOINT ["/app/uvoo-sqvizerver"]
