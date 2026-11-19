FROM node:22-alpine AS console
WORKDIR /src/console
COPY console/package.json console/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY console/ ./
RUN npm run build

FROM golang:1.27.4-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console /src/console/dist ./console/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/moderation ./cmd/moderation

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/moderation /app/moderation
USER nonroot:nonroot
EXPOSE 8090
ENTRYPOINT ["/app/moderation"]
CMD ["serve"]
