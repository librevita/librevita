# Minimal non-root production image. The binary is built on the host or
# CI by `task build` (CGO_ENABLED=0, statically linked, stripped); this
# image only packages it, so there is no build stage.

FROM scratch
COPY bin/librevita /usr/local/bin/librevita
ENV LIBREVITA_DATA_DIR=/data/librevita
VOLUME /data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/librevita"]
