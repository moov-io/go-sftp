FROM golang:1.22-alpine
RUN apk update && apk add shadow
RUN groupadd -g 1000 test && useradd -u 1000 -g 1000 -m -d /usr/src/app test
WORKDIR /usr/src/app
COPY . .
RUN chown -R test:test .
USER test:test
CMD [ "go", "test", "./..." ]
