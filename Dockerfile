# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY web ./
RUN npm run build

FROM golang:1.22-alpine AS go
ARG SQVIZ_BUILD_VERSION=dev
ARG SQVIZ_BUILD_COMMIT=unknown
ARG SQVIZ_BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X uvoo-sqviz/internal/build.Version=${SQVIZ_BUILD_VERSION} -X uvoo-sqviz/internal/build.Commit=${SQVIZ_BUILD_COMMIT} -X uvoo-sqviz/internal/build.Date=${SQVIZ_BUILD_DATE}" -o /out/uvoo-sqvizerver ./cmd/uvoo-sqvizerver

FROM alpine:3.21
ARG SQVIZ_BUILD_VERSION=dev
ARG SQVIZ_BUILD_COMMIT=unknown
ARG SQVIZ_BUILD_DATE=unknown
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 uvoo-sqviz
WORKDIR /app
COPY --from=go /out/uvoo-sqvizerver /app/uvoo-sqvizerver
COPY --from=web /src/web/dist /app/web/dist
USER uvoo-sqviz
ENV SQVIZ_ADDR=:8080 SQVIZ_WEB_ROOT=/app/web/dist SQVIZ_BUILD_VERSION=${SQVIZ_BUILD_VERSION} SQVIZ_BUILD_COMMIT=${SQVIZ_BUILD_COMMIT} SQVIZ_BUILD_DATE=${SQVIZ_BUILD_DATE}
EXPOSE 8080
ENTRYPOINT ["/app/uvoo-sqvizerver"]
