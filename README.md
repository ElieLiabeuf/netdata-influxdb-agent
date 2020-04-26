Netdata-InfluxDB agent
======================

This program is an agent that collect metrics from [Netdata REST API](https://docs.netdata.cloud/web/api/) and write them to [InfluxDB v2](https://v2.docs.influxdata.com/).

The goal is to archive metrics at a lower frequency that they where collected by netdata and provide long term dashboard using [Grafana](https://grafana.com/).

## Why ?

There are many other ways to send Netdata metrics to InfluxDB. You could use [Telegraf](https://github.com/influxdata/telegraf) or [Netdata backends](https://github.com/netdata/netdata/tree/master/backends) for example.

However passing by other protocol like graphite or opentsdb results to a loss of information that may be useful to build complex Grafana dashboards.

A dedicated Netdata input plugin for Telegraf would be the ideal solution but the [PR](https://github.com/influxdata/telegraf/pull/2039) is in standby.

Advantages of this solution versus others
- Standalone, statically linked and light binary
- Sends an writes metrics over HTTP/HTTPS
- Can be hosted anywhere (InfluxDb host, Netdata monitored hosts, somewhere in the middle)
- Communication can be secured if using TLS

## Installation

You can download the binaries directly from the releases section.

A Docker image is also available in the [packages](https://github.com/ElieLiabeuf/netdata-influxdb-agent/packages) section.

See [this readme](./docker/README.md) for documentation.

## Configuration

The agent reads its configuration from a yaml file. Variables are describe in the example file: [net-inf-agent.yml](net-inf-agent.yml).

The `--configuration` flag allow to the give a path to the configuration file.

## Grafana dashboards

Under the [grafana-dashboards](grafana-dashboards) directory you will find ready to use dashboards that can be imported into Grafana.
Provided you have a working InfluxDB v2 setup, a Grafana with [InfluxDB v2 datasource](https://grafana.com/grafana/plugins/grafana-influxdb-flux-datasource) and this agent sending metric to InfluxDB, they should work as is.