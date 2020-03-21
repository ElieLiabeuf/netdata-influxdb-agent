package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/influxdata/influxdb-client-go"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const influxBatchSize = 5000
const chartsURL = "/api/v1/charts"
const infoURL = "/api/v1/info"
const hostPrefixURL = "/host/"

type config struct {
	Netdata Netdata
	Influxdb InfluxDb
}

type InfluxDb struct {
	Url string
	Token string
	Organization string
	Bucket       string

	metric chan influxdb.Metric
	client *influxdb.Client
}

func (inf *InfluxDb) AddMetric(metric influxdb.Metric) {
	select {
	case inf.metric <- metric:
	default:
		log.Errorf("Error when collecting metric, write to influxdb is too slow, dropping metric. Channel metric capcity is %d", cap(inf.metric))
	}
}

func (inf *InfluxDb) dispatch() {
	for {
		batchMetric := make([]influxdb.Metric, 0, influxBatchSize)
		expire := time.After(time.Second)
		for keepGoing := true; keepGoing; {
			select {
			case <- expire:
				keepGoing = false
			case m := <- inf.metric:
				batchMetric = append(batchMetric, m)
				if len(batchMetric) == influxBatchSize {
					keepGoing = false
				}
			}
		}
		if batchSize := len(batchMetric); batchSize > 0 {
			log.Infof("Writing %d measurements to influxdb.", batchSize)
			_, err := inf.client.Write(context.Background(), inf.Bucket, inf.Organization, batchMetric...)
			if err != nil {
				log.Errorf("Error posting influxdb measurements. Error: %s", err)
			}
		}
	}
}

type Netdata struct {
	Servers                 []Server
	Interval				time.Duration
	Points                  int
	Group                   string

	client *http.Client
}

type Server struct {
	Url               string
	BasicAuthUsername string `yaml:"basic_auth_username"`
	BasicAuthPassword string `yaml:"basic_auth_password"`
	lastGatherTime    int64
	currGatherTime    int64
}

type ResponseCharts struct {
	Charts map[string]Chart `json:"charts"`
}

type ServerInfo struct {
	Hosts []string `json:"mirrored_hosts"`
}

type Chart struct {
	ID         string                    `json:"id"`
	Type       string                    `json:"type"`
	Family     string                    `json:"family"`
	DataURL    string                    `json:"data_url"`
	Dimensions map[string]ChartDimension `json:"dimensions"`
}

type ChartDimension struct {
	Name string `json:"name"`
}

type ResponseData struct {
	DimensionNames []string   `json:"dimension_names"`
	Result         DataResult `json:"result"`
}

type DataResult struct {
	Data [][]interface{} `json:"data"`
}

func (n *Netdata) gather(acc *InfluxDb) error {
	var wg sync.WaitGroup

	for idx := range n.Servers {
		wg.Add(1)
		go func(server *Server) {
			defer wg.Done()
			n.gatherServer(acc, server)
		}(&n.Servers[idx])
	}

	wg.Wait()
	return nil
}

func (n *Netdata) gatherServer(acc *InfluxDb, server *Server) {
	server.currGatherTime = time.Now().Unix()

	infoRequestURL, err := url.Parse(server.Url + infoURL)
	if err != nil {
		log.Errorf("Error parsing info url: %s. Error: %s", server.Url+chartsURL, err)
		return
	}

	infoRespBody, err := n.sendRequest(infoRequestURL, server)
	if err != nil {
		log.Errorf("Error getting netdata info, url: %s. Error: %s", infoRequestURL.String(), err)
		return
	}

	var infoResp ServerInfo
	if err := json.Unmarshal([]byte(infoRespBody), &infoResp); err != nil {
		log.Errorf("Error unmarshalling netdata info. Error: %s", err)
		return
	}

	for _, host := range infoResp.Hosts {
		n.gatherHost(acc, server, host)
	}

	server.lastGatherTime = server.currGatherTime
}

func (n *Netdata) gatherHost(acc *InfluxDb, server *Server, host string) {

	chartRequestURL, err := url.Parse(server.Url + hostPrefixURL + host + chartsURL)
	if err != nil {
		log.Errorf("Error parsing charts url: %s. Error: %s", server.Url+hostPrefixURL+host+chartsURL, err)
		return
	}

	chartRespBody, err := n.sendRequest(chartRequestURL, server)
	if err != nil {
		log.Errorf("Error getting netdata charts, url: %s. Error: %s", chartRequestURL.String(), err)
		return
	}

	var chartResp ResponseCharts
	if err := json.Unmarshal([]byte(chartRespBody), &chartResp); err != nil {
		log.Errorf("Error unmarshalling netdata charts. Error: %s", err)
		return
	}

	for _, chart := range chartResp.Charts {
		dataStringURL := n.buildChartDataURL(server, chart, host)

		dataRequestURL, err := url.Parse(dataStringURL)

		if err != nil {
			log.Errorf("Error parsing chart data url: %s. Error: %s", dataStringURL, err)
			continue
		}

		dataRespBody, err := n.sendRequest(dataRequestURL, server)
		if err != nil {
			log.Errorf("Error getting netdata chart data, url: %s. Error: %s", dataRequestURL.String(), err)
			continue
		}

		var dataResp ResponseData
		if err := json.Unmarshal([]byte(dataRespBody), &dataResp); err != nil {
			log.Errorf("Error unmarshalling netdata chart data. Error: %s", err)
			continue
		}

		dimensions := strings.Join(dataResp.DimensionNames, ",")

		measurement := "netdata." + host + "." + chart.ID

		for _, chartData := range dataResp.Result.Data {
			time := time.Unix(int64(chartData[0].(float64)), 0)
			values := chartData[1:]
			fields := make([]*influxdb.Field, 0, len(dataResp.DimensionNames))
			var d string
			for i, dimension := range dataResp.DimensionNames {
				if values[i] != nil {
					if dimension == "time" {
						d = "xtime"
					} else {
						d = dimension
					}
					f := &influxdb.Field{Key: d, Value: values[i]}
					fields = append(fields, f)
				}
			}

			if len(fields) == 0 {
				continue
			}

			tags := []*influxdb.Tag{
				{"server", chartRequestURL.Host},
				{"hostname", host},
				{"type", chart.Type},
				{"family", chart.Family},
				{"dimensions", dimensions},
			}

			m := &influxdb.RowMetric{
				NameStr: measurement,
				Tags:    tags,
				Fields:  fields,
				TS:      time,
			}

			m.SortFields()
			m.SortTags()

			acc.AddMetric(m)
		}
	}
}

func (n *Netdata) buildChartDataURL(server *Server, chart Chart, host string) string {
	return server.Url +
		hostPrefixURL +
		host +
		chart.DataURL +
		"&group=" + n.Group +
		"&options=absolute|jsonwrap|seconds&after=" +
		strconv.FormatInt(server.lastGatherTime, 10) +
		"&before=" +
		strconv.FormatInt(server.currGatherTime, 10) +
		"&points=" +
		strconv.Itoa(n.Points)
}

func (n *Netdata) sendRequest(url *url.URL, server *Server) (string, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	content := bytes.NewBufferString("")
	if server.BasicAuthPassword != "" && server.BasicAuthUsername != "" {
		headers["Authorization"] = "Basic " + base64.URLEncoding.EncodeToString([]byte(server.BasicAuthUsername+":"+server.BasicAuthPassword))
	}
	req, err := http.NewRequest("GET", url.String(), content)
	if err != nil {
		return "", err
	}

	for k, v := range headers {
		req.Header.Add(k, v)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return string(body), err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("Response from url \"%s\" has status code %d (%s), expected %d (%s)",
			url.String(),
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
			http.StatusOK,
			http.StatusText(http.StatusOK))
		return string(body), err
	}
	return string(body), err
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "configuration", "net-inf-agent.yml", "Path to configuration file")
	flag.Parse()

	log.Infof("Reading configuration file: %s", configFile)

	f, err := os.Open(configFile)
	if err != nil {
		log.Errorf("Error when reading configuration file, exiting. Error: %s", err)
		return
	}
	defer f.Close()

	var config config
	if err := yaml.NewDecoder(f).Decode(&config); err != nil {
		log.Errorf("Error when decoding configuration file, exiting. Error: %s", err)
		return
	}
	netdata := &config.Netdata
	influx := &config.Influxdb

	cli := &http.Client{
		Timeout:   2 * time.Second,
	}

	influxCli, err := influxdb.New(influx.Url, influx.Token, influxdb.WithHTTPClient(cli))
	if err != nil {
		log.Errorf("Error when creating influxdb client, exiting. Error: %s", err)
		return
	}
	defer influxCli.Close()

	influx.metric = make(chan influxdb.Metric, influxBatchSize * 1000)
	influx.client = influxCli

	go influx.dispatch()

	netdata.client = cli
	now := time.Now().Unix()
	for idx := range netdata.Servers {
		netdata.Servers[idx].lastGatherTime = now
	}

	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	ticker := time.NewTicker(netdata.Interval)

	for {
		select {
		case <-ticker.C:
			log.Info("Gathering data...")
			start := time.Now()
			netdata.gather(influx)
			log.Infof("Gathering done in %s", time.Since(start))
		case <-signals:
			log.Info("Exiting")
			ticker.Stop()
			return
		}
	}
}
