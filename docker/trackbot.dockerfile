FROM golang:1.15-alpine AS build

WORKDIR /src/
COPY . /src/
WORKDIR /src/cmd/track-bot
RUN go build -a -o /bin/track-bot 

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=build /src/experts.csv /experts.csv
COPY --from=build /bin/track-bot /bin/track-bot 
ENTRYPOINT ["/bin/track-bot", "-experts", "/experts.csv"]
