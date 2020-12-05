FROM golang:1.15-alpine AS build

WORKDIR /src/
COPY . /src/
WORKDIR /src/cmd/vet-bot
RUN CGO_ENABLED=0 go build -a -o /bin/vet-bot 

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=build /src/repos.csv /repos.csv
COPY --from=build /bin/vet-bot  /bin/vet-bot 
ENTRYPOINT ["/bin/vet-bot", "-repos", "/repos.csv"]