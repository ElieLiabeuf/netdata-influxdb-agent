#!/bin/sh -e

j2 /etc/netdata-influxdb-relay/net-inf-agent.yml.j2 > /etc/netdata-influxdb-relay/net-inf-agent.yml

exec "$@"