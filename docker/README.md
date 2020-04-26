## Docker configuration

This image use the following env variables :

- NETDATA_INTERVAL
- NETDATA_POINTS
- NETDATA_GROUP
- NETDATA_SERVERS
- INFLUXDB_URL
- INFLUXDB_TOKEN
- INFLUXDB_ORGANIZATION
- INFLUXDB_BUCKET

See the description in the example file: [net-inf-agent.yml](../net-inf-agent.yml)

multiple servers can be set in NETDATA_SERVERS using a `,` as a separator. Basic Auth can be provided using a `@` before the server url. Login and password use `:` as a separator.
Example : `admin:admin@https://netdata.test,http://example.com`