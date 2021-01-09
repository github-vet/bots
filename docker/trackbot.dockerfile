FROM golang:1.15-alpine AS build

RUN apk update
RUN apk add --no-cache build-base

WORKDIR /src/
COPY . /src/
WORKDIR /src/cmd/track-bot
RUN CGO_ENABLED=1 GOOS=linux go build -a -o /bin/track-bot 

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=build /src/experts.csv /experts.csv
COPY --from=build /bin/track-bot /bin/track-bot 
ENTRYPOINT ["/bin/track-bot", "-experts", "/experts.csv"]
