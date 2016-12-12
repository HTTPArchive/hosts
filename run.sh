#!/bin/bash

if [ -n "$1" ]; then
  TIMESTAMP=$1
else
  TIMESTAMP=$(date "+%Y%m%d")
fi

echo "Starting processing for $TIMESTAMP"

BASE=`pwd`
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

echo -e "Cleaning and splitting host list."
mkdir -p tmp && cd tmp
zcat ../top-1m.csv.zip | cut -d, -f2 | split --lines=100000 - split-

cd $BASE
export GOPATH=~/hosts
ulimit -n 50000
go build

if [ ! -f "$DATA/hosts.json.gz" ]; then
  # cremove old scan results, if present.
  rm $DATA/hosts.json

  for f in $DATA/tmp/split-*
  do
    echo "Processing $f host file..."
    ./hosts -workers=500 -output=$DATA/hosts.json < $f 2> /var/log/HA-host-crawl.log
    if [ $? -ne 0 ]; then
      echo "Host scanner failed, exiting."
      exit
    fi
  done

  echo -e "Compressing host scan results..."
  pigz $DATA/hosts.json
else
 echo -e "Host scanner finished, skipping."
fi

if [ ! -f "done" ]; then
  echo "Starting data join process..."
  ruby process.rb -a $DATA/top-1m.csv.zip -d $DATA/content.rdf.u8.gz -s $DATA/hosts.json.gz > $DATA/joined.json 2> /var/log/HA-host-join.log

  if [ $? -ne 0 ]; then
    echo "Data join failed, exiting."
    exit
  fi
else
 echo -e "Data join finished, skipping."
fi

cd $DATA

echo -e "Syncing data to Google Storage..."
echo "" > "done"
gsutil cp -n * gs://httparchive/urls/${TIMESTAMP}/

echo -e "Kicking off Dataflow pipeline..."
cd $BASE
python dataflow.py --input gs://httparchive/urls/$TIMESTAMP/joined.json --output httparchive:urls.$TIMESTAMP

echo -e "Done."
