#!/bin/bash

set -ex

# if current branch is not master, this PR is not merged to master yet
if [ "$BUILDKITE_BRANCH" != "master" ]; then
    echo "Skipping docker-push since this change is not in master yet"
    exit 0
fi

MASTER_SHA=$(git rev-parse origin/master)

# if this commit is not the same as tip of master, lets skip this and
# let the commit at tip of master do the push
if [ "$BUILDKITE_COMMIT" != "$MASTER_SHA" ]; then
    echo "Skipping docker-push for this commit since tip of master is already ahead"
    exit 0
fi

cd docker

echo "Building docker image for $BUILDKITE_MESSAGE"

docker build . -f Dockerfile -t ubercadence/server:master --build-arg git_branch=$BUILDKITE_COMMIT
docker push ubercadence/server:master

docker build . -f Dockerfile-cli -t ubercadence/cli:master --build-arg git_branch=$BUILDKITE_COMMIT
docker push ubercadence/cli:master
