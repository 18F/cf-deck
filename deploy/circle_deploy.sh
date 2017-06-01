#!/bin/bash

# This script will install the autopilot plugin, login, pick the right manifest and deploy the app with 0 downtime.

set -e
set -o pipefail

# Install cf cli
curl -v -L -o cf-cli_amd64.deb 'https://cli.run.pivotal.io/stable?release=debian64&source=github'
sudo dpkg -i cf-cli_amd64.deb
cf -v

# Install autopilot plugin for blue-green deploys
go get github.com/contraband/autopilot
cf install-plugin -f /home/ubuntu/.go_workspace/bin/autopilot

# Note: Spaces and deployer account username are the same in different environments.
# Only the organization, api, deployer account password differ.



if [[ "$CIRCLE_TAG" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9-]+)? ]]
then
	CF_MANIFEST="manifest-prod.yml"
	CF_SPACE="dashboard-prod"
	CF_APP="cg-dashboard"
elif [ "$CIRCLE_BRANCH" == "master" ]
then
	CF_MANIFEST="manifest-staging.yml"
	CF_SPACE="dashboard-stage"
	CF_APP="cg-dashboard-staging"
elif [ "$CIRCLE_BRANCH" == "demo" ]
then
	CF_MANIFEST="manifest-demo.yml"
	CF_SPACE="dashboard-stage"
	CF_APP="cg-dashboard-demo"
else
  echo Unknown environment, quitting. >&2
  exit 1
fi

# We use the deployer-account broker to get the credentials of
# our deployer accounts.
# Currently, the deployer accounts are scoped to a single space.
# As a result, we will filter by space for which credentials to use.
if [ "$CF_SPACE" == "dashboard-prod" ]
then
	CF_USERNAME=$CF_USERNAME_PROD_SPACE
	CF_PASSWORD=$CF_PASSWORD_PROD_SPACE
elif [ "$CF_SPACE" == "dashboard-stage" ]
then
	CF_USERNAME=$CF_USERNAME_STAGE_SPACE
	CF_PASSWORD=$CF_PASSWORD_STAGE_SPACE
else
	echo "Unknown space. Do not know how to deploy $CF_SPACE."
	exit 1
fi

echo manifest: $CF_MANIFEST
echo space:    $CF_SPACE

function deploy () {
  local manifest=${1}
  local org=${2}
  local space=${3}
  local app=${4}

  # Log in
  cf api $CF_API
  cf auth $CF_USERNAME $CF_PASSWORD
  cf target -o $org -s $space

  # Run autopilot plugin
  cf zero-downtime-push $app -f $manifest
}

# Set manifest path
MANIFEST_PATH=manifests/$CF_MANIFEST
deploy "$MANIFEST_PATH" "$CF_ORGANIZATION" "$CF_SPACE" "$CF_APP"
