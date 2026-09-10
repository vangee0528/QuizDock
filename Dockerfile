# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web ./
COPY internal/webui/dist /src/internal/webui/dist
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /quizdock ./cmd/quizdock \
    && mkdir -p /empty-data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /quizdock /usr/local/bin/quizdock
COPY --chown=nonroot:nonroot --from=build /empty-data /data
VOLUME ["/data"]
EXPOSE 8765
ENTRYPOINT ["/usr/local/bin/quizdock"]
CMD ["serve", "--host", "0.0.0.0", "--data-dir", "/data", "--no-browser"]
