FROM alpine:3.13
RUN apk add --no-cache libc6-compat
WORKDIR /app
COPY ./goosebin .
CMD ["/app/goosebin"]
