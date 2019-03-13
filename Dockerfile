FROM golang:1.12 AS build
WORKDIR /go/src/github.com/heptio/contour

RUN go get github.com/golang/dep/cmd/dep
COPY Gopkg.toml Gopkg.lock ./
RUN dep ensure -v -vendor-only

COPY cmd cmd
COPY internal internal
COPY apis apis
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-ldflags=-w go build -o /go/bin/contour -ldflags=-s -v github.com/heptio/contour/cmd/contour

FROM scratch AS final
COPY --from=build /go/bin/contour /bin/contour
