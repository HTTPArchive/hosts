
"""
  A workflow to process the URL scan & join results.
"""

from __future__ import absolute_import

import argparse
import datetime
import logging
import json

import apache_beam as beam
from apache_beam.utils.options import PipelineOptions
from apache_beam.utils.options import SetupOptions
from apache_beam.utils.options import StandardOptions
from apache_beam.utils.options import GoogleCloudOptions
from apache_beam.utils.options import WorkerOptions
from apache_beam.internal.clients import bigquery
from apache_beam.coders import Coder

# https://github.com/golang/go/blob/master/src/crypto/tls/common.go#L25-L28
TLS_VERSIONS = {
    0x0300: "SSL 3.0",
    0x0301: "TLS 1.0",
    0x0302: "TLS 1.1",
    0x0303: "TLS 1.2"
}

# https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L369-L390
CIPHER_SUITES = {
  0x0005: "TLS_RSA_WITH_RC4_128_SHA",
  0x000a: "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
  0x002f: "TLS_RSA_WITH_AES_128_CBC_SHA",
  0x0035: "TLS_RSA_WITH_AES_256_CBC_SHA",
  0x003c: "TLS_RSA_WITH_AES_128_CBC_SHA256",
  0x009c: "TLS_RSA_WITH_AES_128_GCM_SHA256",
  0x009d: "TLS_RSA_WITH_AES_256_GCM_SHA384",
  0xc007: "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA",
  0xc009: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
  0xc00a: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
  0xc011: "TLS_ECDHE_RSA_WITH_RC4_128_SHA",
  0xc012: "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA",
  0xc013: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
  0xc014: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
  0xc023: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
  0xc027: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
  0xc02f: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
  0xc02b: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
  0xc030: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
  0xc02c: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
  0xcca8: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
  0xcca9: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
}

def process_TLS(record):
  data = record['TLS']
  if data is None:
    return

  record['TLS'] = {
    'HandshakeComplete': data['HandshakeComplete'],
    'NegotiatedProtocol': data['NegotiatedProtocol'],
    'ServerName': data['ServerName'],
    'Version': TLS_VERSIONS.get(data['Version'], 'unknown'),
    'CipherSuite': CIPHER_SUITES.get(data['CipherSuite'], 'unknown')
  }

def process_headers(record):
  headers = record['Headers']
  record['Headers'] = []

  for name, values in headers.iteritems():
    record['Headers'].append({'Name': name, 'Value': values})

def process_record(record):
  """Example input record:
    https://gist.github.com/igrigorik/97835410c5daee52bc2d4272fc097827
  """
  if record['HTTPResponses']:
    for req in record['HTTPResponses']:
      process_headers(req)
      process_TLS(req)

  if record['HTTPSResponses']:
    for req in record['HTTPSResponses']:
      process_headers(req)
      process_TLS(req)

  yield record

class JsonCoder(Coder):
  """A JSON coder interpreting each line as a JSON string."""

  def encode(self, x):
    return json.dumps(x)

  def decode(self, x):
    return json.loads(x)

def field(name, kind='string', mode='nullable'):
  f = bigquery.TableFieldSchema()
  f.name = name
  f.type = kind
  f.mode = mode

  return f

def build_response_schema(name):
  rsp = field(name, 'record', 'repeated')
  rsp.fields.append(field('RequestURL'))
  rsp.fields.append(field('Status', 'integer'))
  rsp.fields.append(field('Protocol'))

  head = field('Headers', 'record', 'repeated')
  head.fields.append(field('Name'))
  head.fields.append(field('Value', 'string', 'repeated'))

  tls = field('TLS', 'record')
  tls.fields.append(field('CipherSuite'))
  tls.fields.append(field('ServerName'))
  tls.fields.append(field('HandshakeComplete', 'boolean'))
  tls.fields.append(field('Version'))
  tls.fields.append(field('NegotiatedProtocol'))

  rsp.fields.append(head)
  rsp.fields.append(tls)
  return rsp

def run(argv=None):
  """Runs the workflow."""

  parser = argparse.ArgumentParser()
  parser.add_argument('--input',
                      required=True,
                      help='Input file to process.')
  parser.add_argument('--output',
                      required=True,
                      help='Output BigQuery table: PROJECT:DATASET.TABLE')
  known_args, pipeline_args = parser.parse_known_args(argv)

  schema = bigquery.TableSchema()
  schema.fields.append(field('Alexa_rank', 'integer'))
  schema.fields.append(field('Alexa_domain'))

  schema.fields.append(field('DMOZ_title'))
  schema.fields.append(field('DMOZ_description'))
  schema.fields.append(field('DMOZ_url'))
  schema.fields.append(field('DMOZ_topic', 'string', 'repeated'))

  schema.fields.append(field('Host'))
  schema.fields.append(field('FinalLocation'))
  schema.fields.append(field('HTTPOk', 'boolean'))
  schema.fields.append(field('HTTPSOk', 'boolean'))
  schema.fields.append(field('HTTPSOnly', 'boolean'))

  schema.fields.append(build_response_schema('HTTPResponses'))
  schema.fields.append(build_response_schema('HTTPSResponses'))
  schema.fields.append(field('Error'))

  options = PipelineOptions(pipeline_args)
  # We use the save_main_session option because one or more DoFn's in this
  # workflow rely on global context (e.g., a module imported at module level).
  options.view_as(SetupOptions).save_main_session = True

  # https://cloud.google.com/dataflow/pipelines/specifying-exec-params
  gc_options = options.view_as(GoogleCloudOptions)
  gc_options.project = 'httparchive'
  gc_options.job_name = 'host-scan-import-' + str(datetime.date.today())
  gc_options.staging_location = 'gs://httparchive/dataflow-binaries'
  gc_options.temp_location = 'gs://httparchive/dataflow-tmp'

  wk_options = options.view_as(WorkerOptions)
  wk_options.num_workers = 10

  # options.view_as(StandardOptions).runner = 'DirectPipelineRunner'
  options.view_as(StandardOptions).runner = 'DataflowPipelineRunner'

  p = beam.Pipeline(options=options)
  (p
   | 'read' >> beam.Read(
      beam.io.TextFileSource(known_args.input, coder=JsonCoder()))
   | 'process'  >> beam.FlatMap(process_record)
   # | 'local-write' >> beam.Write(beam.io.TextFileSink('./results')))
   | 'bq-write' >> beam.io.Write(
      beam.io.BigQuerySink(
        known_args.output,
        schema=schema,
        create_disposition=beam.io.BigQueryDisposition.CREATE_IF_NEEDED,
        write_disposition=beam.io.BigQueryDisposition.WRITE_TRUNCATE)
      )
    )
  p.run()

if __name__ == '__main__':
  logging.getLogger().setLevel(logging.INFO)
  run()
