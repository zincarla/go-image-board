FROM alpine:latest
RUN mkdir "/var/go-image-board"
RUN mkdir "/var/go-image-board/images"
RUN mkdir "/var/go-image-board/images/thumbs"
RUN mkdir "/var/go-image-board/configuration"
RUN chmod 766 "/var/go-image-board"
COPY gib "/var/go-image-board/"
COPY http "/var/go-image-board/http"
RUN chmod +x "/var/go-image-board/gib"
EXPOSE 8080
WORKDIR /var/go-image-board
ENTRYPOINT ["/var/go-image-board/gib"]