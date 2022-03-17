FROM golang:alpine as builder

RUN apk add git

WORKDIR /app

COPY go.* ./
RUN go mod download

COPY . ./

RUN go build -v

FROM alpine

RUN apk update && apk upgrade

RUN adduser -D sensoruser

COPY --from=builder /app/eventsrunner-k8s-sensor /usr/local/bin/eventsrunner-k8s-sensor

USER sensoruser

ENTRYPOINT [ "eventsrunner-k8s-sensor" ]

CMD [ "eventsrunner-k8s-sensor" ]
