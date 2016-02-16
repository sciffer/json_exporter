# json_exporter

<p>A generic json exporter for prometheus.io monitoring solution, the exporter will extract all the numeric values from the json and turn them into metrics, booleans will be translated(1 - true, 0 - false) and strings will be ignored. The exporter will try to parse through the whole json and array depths while the metric names will include the hierarchy with _ as seperator.</p>

json_exporter [-debug] [-j.labels label1[,label2[..labelN]]] [-j.values label1value[,label2value[..labelNvalue]]] [-j.timeout <timeout>] [-namespace <namespace>] [-web.listen-address <listening address>] [-web.telemetry-path <telemetry path>] <url1>[ <url2>[.. <urlN>]]

**Url list** : The list of non flagged parameters will be treated as URL's

**-j.labels & -j.values** : are parallel to one each other (the first label will have the first value, etc..) - same as urls a list of labels and values are comma seperated.


Example of how one can use the exporter to monitor elasticsearch:
>json_exporter -namespace elasticsearch -j.labels cluster,datacenter -j.values es1,dc1 http://localhost:9200/_cluster/health http://localhost:9200/_cluster/stats

Notes:
*All values currently are considered by default guage as there is no way for me to determine if something is counter of gauge(it's generic).
