#!/bin/bash

set -e

VETBOT_IMAGE=vetbot
TRACKBOT_IMAGE=trackbot

docker build -t gcr.io/${PROJECT_PROD}/${VETBOT_IMAGE}:$TRAVIS_COMMIT -f docker/vetbot.dockerfile .
docker build -t gcr.io/${PROJECT_PROD}/${TRACKBOT_IMAGE}:$TRAVIS_COMMIT -f docker/trackbot.dockerfile .

echo $GCLOUD_SERVICE_KEY_PROD | base64 --decode -i > ${HOME}/gcloud-service-key.json
gcloud auth activate-service-account --key-file ${HOME}/gcloud-service-key.json

gcloud --quiet config set project $PROJECT_PROD
gcloud --quiet config set container/cluster $CLUSTER
gcloud --quiet config set compute/zone ${ZONE}
gcloud --quiet container clusters get-credentials $CLUSTER

gcloud docker -- push gcr.io/${PROJECT_PROD}/${VETBOT_IMAGE}
gcloud docker -- push gcr.io/${PROJECT_PROD}/${TRACKBOT_IMAGE}

yes | gcloud beta container images add-tag gcr.io/${PROJECT_PROD}/${VETBOT_IMAGE}:$TRAVIS_COMMIT gcr.io/${PROJECT_PROD}/${VETBOT_IMAGE}:latest
yes | gcloud beta container images add-tag gcr.io/${PROJECT_PROD}/${TRACKBOT_IMAGE}:$TRAVIS_COMMIT gcr.io/${PROJECT_PROD}/${TRACKBOT_IMAGE}:latest
