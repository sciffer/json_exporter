# json_exporter

<p>A generic json exporter for prometheus.io monitoring solution, the exporter will extract all the numeric values from the json and turn them into metrics, booleans will be translated(1 - true, 0 - false) and strings will be ignored. The exporter will try to parse through the whole json and array depths while the metric names will include the hierarchy with _ as seperator.</p>

json_exporter [-lowercase] [-jmx] [-debug] [-valuelabels name:regex[ /<..>]] [-pathlabels name:regex[ /<..>]] [-labels label1[,label2[..labelN]]] [-values label1value[,label2value[..labelNvalue]]] [-timeout timeout] [-interval minimum refresh interval] [-namespace namespace] [-web.listen-address listening address] [-web.telemetry-path "telemetry path"] [-blacklist regex>] [-whitelist regex] url1[ url2[.. urlN]]

**Url list** : The list of non flagged parameters will be treated as URL's

**-labels & -values** : are parallel to one each other (the first label will have the first value, etc..) - same as urls a list of labels and values are comma seperated.

**-valuelabel** : labels which their values are extracted from metric values, runs once upon startup and turns the value of the first match into a label value with it's relevant name.

**-pathlabel** : labels will be created from the path (to the relevant childs scope only) with the specified name, the  metric prefix name will change to the label name as well and the value will not be part of the metric path. The value within the regex should have brackets around it.

**-jmx** : JMX mode is for translating the "name" within the json array into a path, since JMX JSON format is only an array of json without hierarchy to define the path like in a structured JSON.

Example of how one can use the exporter to monitor elasticsearch:
>json_exporter -namespace elasticsearch -labels datacenter -values dc1 -valuelabel='cluster:^cluster_name$' -pathlabel='node:^nodes_([a-zA-Z0-9]{22})$/index:^indices_([a-zA-Z0-9-.])*$' http://localhost:9200/_cluster/health http://localhost:9200/_stats http://localhost:9200/_nodes/stats (running this command on a single machine in the cluster will provide metrics for the whole cluster - and a lot of them - so apply your own blacklist/whitelist combination to get the once you need).

Notes:
* All values currently are considered by default guage as there is no way for me to determine if something is counter of gauge(it's generic).
* Pathlabel option change the metric name upon match, please make sure to test carefully before applying in production.
