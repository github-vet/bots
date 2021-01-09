FROM golang:1.15-alpine AS build

RUN apk update
RUN apk add --no-cache build-base

WORKDIR /src/
COPY . /src/
WORKDIR /src/cmd/vet-bot
RUN go build -a -o /bin/vet-bot 

FROM alpine
RUN apk --no-cache add ca-certificates

COPY --from=build /src/repos.csv /bootstrap/repo_seed.csv
COPY --from=build /src/internal/db/bootstrap /bootstrap
COPY --from=build /bin/vet-bot  /bin/vet-bot 
ENTRYPOINT ["/bin/vet-bot", "-schemas", "/bootstrap"]
