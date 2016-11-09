# Host scanner
Ingests a list of hostnames and runs the following checks:

- Fetches the http:// site and checks for timeouts & errors
- Fetches the https:// site
- Records:
    - Results of the TLS handshake, if applicable
    - Final redirect destination based on above
    - "HTTPS-only" status


# Installing & Running
Install Go Version Manager: https://github.com/moovweb/gvm

```bash
 $> gvm install 1.7.3 (+)
 $> go build

 # run with custom number of workers and output file
 $> ./hosts -workers=10 -output=results.json < /path/to/list-of-domains 2> output.log
 $> unzip -p list-of-domains.zip | DEBUG=true ./hosts -workers=2

 # run with verbose debug output
 $> DEBUG=true ./hosts -workers=10 -output=results.json < /path/to/list-of-domains 2> output.log
 ```

# Dataflow pipeline

1. Follow steps to [install Apache Beam SDK](https://github.com/apache/incubator-beam/tree/python-sdk/sdks/python#get-apache-beam).
2. Activate environment and run the dataflow job:

```
$> pip install google-cloud-dataflow
$> virtualenv /path/to/dataflow-env
$> . /path/to/dataflow-env/bin/activate
$> python dataflow.py  --input gs://httparchive/urls/<input-file> --output project:dataset.tablename
```
