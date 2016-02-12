package main

import (
	"encoding/json"
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxGauges   = 1024
	maxCounters = 1024
)

// Exporter collects Elasticsearch stats from the given server and exports
// them using the prometheus metrics package.
type Exporter struct {
	Urls        []string
	namespace   string
	labels      []string
	labelvalues []string
	mutex       sync.RWMutex

	up prometheus.Gauge

	gauges   []*prometheus.GaugeVec
	counters []*prometheus.CounterVec

	client *http.Client
}

// NewExporter returns an initialized Exporter.
func JsonExporter(urls []string, timeout time.Duration, namespace string, labels []string, labelvalues []string) *Exporter {
	counters := make([]*prometheus.CounterVec, 0, maxCounters)
	gauges := make([]*prometheus.GaugeVec, 0, maxGauges)

	// Init our exporter.
	return &Exporter{
		Urls:        urls,
		namespace:   namespace,
		labels:      labels,
		labelvalues: labelvalues,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the json query successful?",
		}),

		counters: counters,
		gauges:   gauges,

		client: &http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					c, err := net.DialTimeout(netw, addr, timeout)
					if err != nil {
						return nil, err
					}
					if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
						return nil, err
					}
					return c, nil
				},
			},
		},
	}
}

// Describe describes all the metrics ever exported by the elasticsearch
// exporter. It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up.Desc()

	for _, c := range e.counters {
		c.Describe(ch)
	}

	for _, g := range e.gauges {
		g.Describe(ch)
	}

}

// Adding single gauge metric to the slice
func (e *Exporter) addGauge(name string, value float64, help string) {
	if len(e.gauges) < cap(e.gauges) {
		curr := len(e.gauges)
		e.gauges = e.gauges[:(curr + 1)]
		e.gauges[curr] = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: e.namespace, Name: name, Help: help}, e.labels)
		e.gauges[curr].WithLabelValues(e.labelvalues...).Set(value)
	} else {
		log.Println("Max gauges limit reached:", cap(e.gauges))
	}
}

// Extract metrics of generic json interface
// push extracted metrics accordingly (to guages only at the moment)
func (e *Exporter) extractJson(metric string, jsonInt map[string]interface{}) {
	if metric != "" {
		metric = metric + "_"
	}
	for k, v := range jsonInt {
		switch vv := v.(type) {
		case string:
			log.Println(metric, k, "is string", vv)
		case int:
			log.Println(metric, k, "is int =>", vv)
			e.addGauge(metric+k, float64(vv), "")
		case float64:
			log.Println(metric, k, "is float64 =>", vv)
			e.addGauge(metric+k, vv, "")
		case bool:
			if vv {
				log.Println(metric, k, "is bool => 1")
				e.addGauge(metric+k, float64(1), "")
			} else {
				log.Println(metric, k, "is bool => 0")
				e.addGauge(metric+k, float64(0), "")
			}
		case map[string]interface{}:
			newMetric := metric + k
			log.Println(metric, k, "is hash", newMetric)
			e.extractJson(newMetric, vv)
		case []interface{}:
			newMetric := metric + k
			log.Println(k, "is an array:", newMetric)
			e.extractJsonArray(newMetric, vv)
		default:
			log.Println(k, "is of a type I don't know how to handle")
		}
	}
}
func (e *Exporter) extractJsonArray(metric string, jsonInt []interface{}) {
	if metric != "" {
		metric = metric + "_"
	}
	for k, v := range jsonInt {
		switch vv := v.(type) {
		case string:
			log.Println(metric, k, "is string", vv)
		case int:
			log.Println(metric, k, "is int =>", vv)
			e.addGauge(metric+strconv.Itoa(k), float64(vv), "")
		case float64:
			log.Println(metric, k, "is int =>", vv)
			e.addGauge(metric+strconv.Itoa(k), vv, "")
		case bool:
			if vv {
				log.Println(metric, k, "is bool => 1")
				e.addGauge(metric+strconv.Itoa(k), float64(1), "")
			} else {
				log.Println(metric, k, "is bool => 0")
				e.addGauge(metric+strconv.Itoa(k), float64(0), "")
			}
		case map[string]interface{}:
			newMetric := metric + strconv.Itoa(k)
			log.Println(metric, k, "is hash", newMetric)
			e.extractJson(newMetric, vv)
		case []interface{}:
			newMetric := metric + strconv.Itoa(k)
			log.Println(k, "is an array:", newMetric)
			e.extractJsonArray(newMetric, vv)
		default:
			log.Println(k, "is of a type I don't know how to handle")
		}
	}
}

// Collect fetches the stats from configured elasticsearch location and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	// Release previous metrics.
	e.gauges = e.gauges[:0]
	e.counters = e.counters[:0]

	defer func() { ch <- e.up }()

	for _, URI := range e.Urls {
		resp, err := e.client.Get(URI)
		if err != nil {
			e.up.Set(0)
			log.Println("Error while querying Json endpoint:", err)
			return
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println("Failed to read Json response body:", err)
			e.up.Set(0)
			return
		}

		e.up.Set(1)

		var allStats map[string]interface{}
		err = json.Unmarshal(body, &allStats)
		if err != nil {
			log.Println("Failed to unmarshal JSON into struct:", err)
			return
		}

		// Extracrt the metrics from the json interface
		e.extractJson("", allStats)
	}
	// Report metrics.

	for _, c := range e.counters {
		c.Collect(ch)
	}

	for _, g := range e.gauges {
		g.Collect(ch)
	}
}

func main() {
	var (
		listenAddress = flag.String("web.listen-address", ":9109", "Address to listen on for web interface and telemetry.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		jUrls         = flag.String("j.urls", "http://localhost", "List of urls to scrape (comma seperated).")
		jLabels       = flag.String("j.labels", "", "List of labels (comma seperated).")
		jLabelValues  = flag.String("j.values", "", "List of label values (comma seperated)")
		jTimeout      = flag.Duration("j.timeout", 5*time.Second, "Timeout for trying to get to json URI.")
		namespace     = flag.String("namespace", "json", "Namespace for metrics exported from Json.")
	)
	flag.Parse()
	urls := strings.Split(*jUrls, ",")
	log.Println("Got the following Url list", urls)
	labels := strings.Split(*jLabels, ",")
	labelValues := strings.Split(*jLabelValues, ",")
	if len(labels) != len(labelValues) {
		log.Fatal("Labels amount does not match value amount")
	}

	exporter := JsonExporter(urls, *jTimeout, *namespace, labels, labelValues)
	prometheus.MustRegister(exporter)

	log.Println("Starting Server:", *listenAddress)
	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Json Exporter</title></head>
             <body>
             <h1>Json Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
