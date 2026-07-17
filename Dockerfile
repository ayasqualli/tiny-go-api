FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY api ./api
COPY web ./we
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /task-api

FROM scratch
COPY --from=build /task-api /task-api
EXPOSE 3000
ENV PORT=3000
ENTRYPOINT ["/task-api"]