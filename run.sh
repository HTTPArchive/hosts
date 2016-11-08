#!/bin/bash

BASE=`pwd`
TIMESTAMP=$(date "+%Y%m%d")
DATA=$HOME/archive/urls/$TIMESTAMP

mkdir -p $DATA
cd $DATA

echo -e "Fetching Alexa Top 1M archive"
wget -nv -N "http://s3.amazonaws.com/alexa-static/top-1m.csv.zip"
if [ $? -ne 0 ]; then
  echo "Alexa fetch failed, exiting"
  exit
fi

## http://rdf.dmoz.org/
echo -e "Fetching DMOZ open directory RDF dump"
wget -nv -N "http://rdf.dmoz.org/rdf/content.rdf.u8.gz"
if [ $? -ne 0 ]; then
  echo "DMOZ fetch failed, exiting"
  exit
fi

ruby $BASE/process.rb -a top-1m.csv.zip -d content.rdf.u8.gz
if [ $? -ne 0 ]; then
  echo "DMOZ join failed, exiting"
  exit
fi

./hosts -workers=500 -output=hosts.json 2> hosts.log
if [ $? -ne 0 ]; then
  echo "Host scanner failed, exiting"
  exit
fi

pigz hosts.json

echo -e "Syncing data to Google Storage"
gsutil cp -n *.{zip,gz} gs://httparchive/urls/${TIMESTAMP}/

echo "" > "done"
gsutil cp "done" gs://httparchive/urls/${TIMESTAMP}/

echo -e "Done."

