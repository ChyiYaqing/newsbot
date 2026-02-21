FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /newsbot .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /newsbot /usr/local/bin/newsbot

WORKDIR /app
RUN mkdir -p /app/data
EXPOSE 8080

ENTRYPOINT ["newsbot"]
CMD ["run"]
