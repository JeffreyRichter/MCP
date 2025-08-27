FROM mcr.microsoft.com/oss/go/microsoft/golang:1.25-azurelinux3.0 AS builder
COPY . /src
WORKDIR /src/mcpsvc
RUN go mod tidy
RUN go build -tags=goexperiment.jsonv2 -o /build/mcpsvc .

FROM mcr.microsoft.com/azurelinux/base/core:3.0
COPY --from=builder /build/mcpsvc .
RUN chmod +x mcpsvc
EXPOSE 8080
CMD ["./mcpsvc"]
