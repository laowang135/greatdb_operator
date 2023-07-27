# Build the manager binary
FROM docker.io/grafana/promtail:2.7.4
RUN groupadd greatdb --gid 1000 && \
    useradd -r -g greatdb -s /bin/false --uid 1000 greatdb && \
    mkdir /greatdb && \
    chown -R greatdb:greatdb /greatdb
WORKDIR /
COPY ./conf /etc/promtail/


