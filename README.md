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
