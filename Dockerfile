FROM golang:alpine AS buildStage
RUN apk --no-cache add ca-certificates
WORKDIR /line-bot-demo
COPY go.mod go.sum ./
RUN  go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build

FROM scratch
WORKDIR /app
COPY --from=buildStage /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=buildStage /line-bot-demo/line-bot-demo .
COPY --from=buildStage /line-bot-demo/static .
EXPOSE 9203
ENTRYPOINT ["/app/line-bot-demo"]