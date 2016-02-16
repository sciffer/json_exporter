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

// Exporter collects Elasticsearch stats from the given server and exports
// them using the prometheus metrics package.
type Exporter struct {
	Urls        []string
	namespace   string
	labels      []string
	labelvalues []string
	mutex       sync.RWMutex
	debug	    bool

	up prometheus.Gauge

	gauges   map[string]*prometheus.GaugeVec
	updated  map[string]bool

	client *http.Client
}

// NewExporter returns an initialized Exporter.
func JsonExporter(urls []string, timeout time.Duration, namespace string, labels []string, labelvalues []string, debug bool) *Exporter {
	gauges := make(map[string]*prometheus.GaugeVec)
	updated := make(map[string]bool)

	// Init our exporter.
	return &Exporter{
		Urls:        urls,
		namespace:   namespace,
		labels:      labels,
		labelvalues: labelvalues,
		debug:       debug,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the json query successful?",
		}),

		gauges:   gauges,
		updated:  updated,

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

	for _, g := range e.gauges {
		g.Describe(ch)
	}

}

// Adding single gauge metric to the slice
func (e *Exporter) addGauge(name string, value float64, help string) {
	if _, exists := e.gauges[name]; exists {
		e.gauges[name].WithLabelValues(e.labelvalues...).Set(value)
	} else {
		e.gauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: e.namespace, Name: name, Help: help}, e.labels)
		e.gauges[name].WithLabelValues(e.labelvalues...).Set(value)
	}
	e.updated[name] = true
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
			if e.debug {
				log.Println(metric, k, "is string", vv)
			}
			if vv[0] == '{' {
				var stats map[string]interface{}
		                err := json.Unmarshal([]byte(vv), &stats)
				if err != nil {
					log.Println("Failed to parse json from string",metric,k)
				} else {
					if e.debug {
						log.Println("Extracting json values from the string in:",metric,k)
					}
					e.extractJson(metric + k, stats)
				}
			}
		case int:
			if e.debug {
				log.Println(metric, k, "is int =>", vv)
			}
			e.addGauge(metric+k, float64(vv), "")
		case float64:
			if e.debug {
				log.Println(metric, k, "is float64 =>", vv)
			}
			e.addGauge(metric+k, vv, "")
		case bool:
			if vv {
				if e.debug {
					log.Println(metric, k, "is bool => 1")
				}
				e.addGauge(metric+k, float64(1), "")
			} else {
				if e.debug {
					log.Println(metric, k, "is bool => 0")
				}
				e.addGauge(metric+k, float64(0), "")
			}
		case map[string]interface{}:
			newMetric := metric + k
			if e.debug {
				log.Println(metric, k, "is hash", newMetric)
			}
			e.extractJson(newMetric, vv)
		case []interface{}:
			newMetric := metric + k
			if e.debug {
				log.Println(k, "is an array:", newMetric)
			}
			e.extractJsonArray(newMetric, vv)
		default:
			if e.debug {
				log.Println(k, "is of a type I don't know how to handle")
			}
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
			if e.debug {
				log.Println(metric, k, "is string", vv)
			}
			if vv[0] == '{' {
				var stats map[string]interface{}
		                err := json.Unmarshal([]byte(vv), &stats)
				if err != nil {
					log.Println("Failed to parse json from string",metric,k)
				} else {
					e.extractJson(metric + strconv.Itoa(k), stats)
					if e.debug {
						log.Println("Extracting json values from the string in:",metric,k)
					}
				}
			}
		case int:
			if e.debug {
				log.Println(metric, k, "is int =>", vv)
			}
			e.addGauge(metric+strconv.Itoa(k), float64(vv), "")
		case float64:
			if e.debug {
				log.Println(metric, k, "is int =>", vv)
			}
			e.addGauge(metric+strconv.Itoa(k), vv, "")
		case bool:
			if vv {
				if e.debug {
					log.Println(metric, k, "is bool => 1")
				}
				e.addGauge(metric+strconv.Itoa(k), float64(1), "")
			} else {
				if e.debug {
					log.Println(metric, k, "is bool => 0")
				}
				e.addGauge(metric+strconv.Itoa(k), float64(0), "")
			}
		case map[string]interface{}:
			newMetric := metric + strconv.Itoa(k)
			if e.debug {
				log.Println(metric, k, "is hash", newMetric)
			}
			e.extractJson(newMetric, vv)
		case []interface{}:
			newMetric := metric + strconv.Itoa(k)
			if e.debug {
				log.Println(k, "is an array:", newMetric)
			}
			e.extractJsonArray(newMetric, vv)
		default:
			if e.debug {
				log.Println(k, "is of a type I don't know how to handle")
			}
		}
	}
}

// Collect fetches the stats from configured elasticsearch location and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	defer func() { ch <- e.up }()

	for name, updated := range e.updated {
		if !updated {
			//delete metricvec
			delete(e.updated, name)
			//delete updated value
			delete(e.gauges, name)
		} else {
			//reset value
			e.updated[name] = false
		}
	}

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

	for _, g := range e.gauges {
		g.Collect(ch)
	}
}

func main() {
	var (
		listenAddress = flag.String("web.listen-address", ":9109", "Address to listen on for web interface and telemetry.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		jLabels       = flag.String("j.labels", "", "List of labels (comma seperated).")
		jLabelValues  = flag.String("j.values", "", "List of label values (comma seperated)")
		jTimeout      = flag.Duration("j.timeout", 5*time.Second, "Timeout for trying to get to json URI.")
		namespace     = flag.String("namespace", "json", "Namespace for metrics exported from Json.")
		debug         = flag.Bool("debug", false, "Print debug information")
	)
	flag.Parse()
	urls := flag.Args()
	if len(urls) < 1 {
		log.Fatal("Got no URL's, please add use the following syntax to add URL's: json_exporter [options] <URL1>[ <URL2>[ ..<URLn>]]")
	} else {
		log.Println("Got the following Url list", urls)
	}
	labels := strings.Split(*jLabels, ",")
	labelValues := strings.Split(*jLabelValues, ",")
	if len(labels) != len(labelValues) {
		log.Fatal("Labels amount does not match value amount!!!")
	}

	exporter := JsonExporter(urls, *jTimeout, *namespace, labels, labelValues, *debug)
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
