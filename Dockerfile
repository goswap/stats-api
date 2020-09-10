FROM golang:1.15-alpine AS build-env
RUN apk --no-cache add build-base git mercurial gcc
WORKDIR /myapp
# cache dependencies
ADD go.mod /myapp
ADD go.sum /myapp
RUN go mod download
# now build
ADD . /myapp
RUN cd /myapp && go build -o mybin && cp mybin /tmp/

# final stage
FROM alpine
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build-env /tmp/mybin /app/myapp
CMD ["/app/myapp"]
