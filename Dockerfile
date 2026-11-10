FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /rate-limiter ./cmd/server

FROM alpine:3.22

COPY --from=build /rate-limiter /rate-limiter
EXPOSE 8080
CMD ["/rate-limiter"]
