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
	"regexp"
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
	nextrefresh time.Time
	interval    time.Duration

	up prometheus.Gauge

	gauges   map[string]*prometheus.GaugeVec
	updated  map[string]bool

	blacklist *regexp.Regexp
	whitelist *regexp.Regexp

	client *http.Client
}

// NewExporter returns an initialized Exporter.
func JsonExporter(urls []string, timeout time.Duration, namespace string, labels []string, labelvalues []string, debug bool, blacklist string, whitelist string, refreshinterval time.Duration) *Exporter {
	gauges := make(map[string]*prometheus.GaugeVec)
	updated := make(map[string]bool)
	var blist, wlist *regexp.Regexp
	if blacklist != "" {
		blist = regexp.MustCompile(blacklist)
	}
	if whitelist != "" {
		wlist = regexp.MustCompile(whitelist)
	}

	// Init our exporter.
	return &Exporter{
		Urls:        urls,
		namespace:   namespace,
		labels:      labels,
		labelvalues: labelvalues,
		debug:       debug,
		nextrefresh: time.Now(),
		interval:    refreshinterval,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the json query successful?",
		}),

		gauges:   gauges,
		updated:  updated,

		blacklist: blist,
		whitelist: wlist,

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

// Matching metric names against blacklist/whitelist
func (e *Exporter) matchMetric(name string) bool {
	if (e.blacklist != nil && e.blacklist.MatchString(name)) || (e.whitelist != nil && !e.whitelist.MatchString(name)) {
		return false
	} else {
		return true
	}
}

// Adding single gauge metric to the slice
func (e *Exporter) addGauge(name string, value float64, help string) {
	if _, exists := e.gauges[name]; !exists {
		e.gauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: e.namespace, Name: name, Help: help}, e.labels)
	}
	e.gauges[name].WithLabelValues(e.labelvalues...).Set(value)
	e.updated[name] = true
}

// Extract metrics of generic json interface
// push extracted metrics accordingly (to guages only at the moment)
func (e *Exporter) extractJson(metric string, jsonInt map[string]interface{}) {
	if metric != "" {
		metric = metric + "_"
	}
	for k, v := range jsonInt {
		if e.matchMetric(metric + k) {
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
}
func (e *Exporter) extractJsonArray(metric string, jsonInt []interface{}) {
	if metric != "" {
		metric = metric + "_"
	}
	for k, v := range jsonInt {
		if e.matchMetric(metric + strconv.Itoa(k)) {
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
}

// Collect fetches the stats from configured elasticsearch location and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	defer func() { ch <- e.up }()

	if e.nextrefresh.Before(time.Now()) {
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
		e.nextrefresh = time.Now().Add(e.interval)
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
		Labels        = flag.String("labels", "", "List of labels (comma seperated).")
		LabelValues   = flag.String("values", "", "List of label values (comma seperated)")
		Timeout       = flag.Duration("timeout", 5*time.Second, "Timeout for trying to get to json URI.")
		interval      = flag.Duration("interval", 1*time.Minute, "Refresh interval for json scraping.")
		namespace     = flag.String("namespace", "json", "Namespace for metrics exported from Json.")
		debug         = flag.Bool("debug", false, "Print debug information")
		blacklist     = flag.String("blacklist", "", "Blacklist regex expression of metric names.")
		whitelist     = flag.String("whitelist", "", "Whitelist regex expression of metric names.")
	)
	flag.Parse()
	urls := flag.Args()
	if len(urls) < 1 {
		log.Fatal("Got no URL's, please add use the following syntax to add URL's: json_exporter [options] <URL1>[ <URL2>[ ..<URLn>]]")
	} else {
		log.Println("Got the following Url list", urls)
	}
	labels := strings.Split(*Labels, ",")
	labelValues := strings.Split(*LabelValues, ",")
	if len(labels) != len(labelValues) {
		log.Fatal("Labels amount does not match value amount!!!")
	}

	exporter := JsonExporter(urls, *Timeout, *namespace, labels, labelValues, *debug, *blacklist, *whitelist, *interval)
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
