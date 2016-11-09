#!/bin/bash

BASE=`pwd`
TIMESTAMP=$(date "+%Y%m%d")
DATA=$HOME/archive/urls/$TIMESTAMP

mkdir -p $DATA
cd $DATA

echo -e "Fetching Alexa Top 1M archive..."
if [ ! -f "top-1m.csv.zip" ]; then
  wget -nv -N "http://s3.amazonaws.com/alexa-static/top-1m.csv.zip"
  if [ $? -ne 0 ]; then
    echo "Alexa fetch failed, exiting."
    exit
  fi
else
  echo -e "Alexa data already downloaded, skipping."
fi

if [ ! -f "content.rdf.u8.gz" ]; then
  echo -e "Fetching DMOZ open directory RDF dump..."
  wget -nv -N "http://rdf.dmoz.org/rdf/content.rdf.u8.gz"
  if [ $? -ne 0 ]; then
    echo "DMOZ fetch failed, exiting."
    exit
  fi
else
  echo -e "DMOZ data already downloaded, skipping."
fi

cd $BASE
export GOPATH=~/hosts
ulimit -n 50000
go build

if [ ! -f "$DATA/hosts.json.gz" ]; then
  zcat $DATA/top-1m.csv.zip | cut -d, -f2 | ./hosts -workers=500 -output=$DATA/hosts.json 2> /var/log/HA-host-scanner.log
  if [ $? -ne 0 ]; then
    echo "Host scanner failed, exiting."
    exit
  fi

  echo -e "Compressing host scan results..."
  pigz $DATA/hosts.json
else
 echo -e "Host scanner finished, skipping."
fi

if [ ! -f "done" ]; then
  echo "Starting data join process..."
  ruby process.rb -a $DATA/top-1m.csv.zip -d $DATA/content.rdf.u8.gz -s $DATA/hosts.json.gz 2> /var/log/HA-host-join.log

  if [ $? -ne 0 ]; then
    echo "Data join failed, exiting."
    exit
  fi
else
 echo -e "Data join finished, skipping."
fi

cd $DATA

echo -e "Syncing data to Google Storage..."
gsutil cp -n *.{zip,gz} gs://httparchive/urls/${TIMESTAMP}/

echo "" > "done"
gsutil cp "done" gs://httparchive/urls/${TIMESTAMP}/

echo -e "Done."
