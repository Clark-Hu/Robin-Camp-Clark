 

##########
# Builder
##########
FROM golang:1.22 AS builder
WORKDIR /src

ENV GOPROXY=https://goproxy.cn,direct


COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/movies-api ./cmd/server

##########
# Runtime
##########
FROM gcr.io/distroless/base-debian12 AS runtime
WORKDIR /app

COPY --from=builder /bin/movies-api /app/movies-api

# port will be allocated in compose.yml
# ENV PORT=8080
# EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/movies-api"]
