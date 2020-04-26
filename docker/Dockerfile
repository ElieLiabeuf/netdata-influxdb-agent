FROM alpine:3
ARG version

RUN apk add --no-cache py-pip && pip install j2cli

ADD https://github.com/ElieLiabeuf/netdata-influxdb-agent/releases/download/$version/netdata-influxdb-agent-amd64 /usr/bin/
RUN chmod u+x /usr/bin/netdata-influxdb-agent-amd64

COPY docker-entrypoint.sh /usr/bin/
COPY net-inf-agent.yml.j2 /etc/netdata-influxdb-agent/

ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
CMD ["netdata-influxdb-agent-amd64 -c /etc/netdata-influxdb-agent/net-inf-agent.yml"]
