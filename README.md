# pdns_janitor

[![Create and publish a Docker image](https://github.com/opensvc/pdns_janitor/actions/workflows/docker-build-publish.yaml/badge.svg)](https://github.com/opensvc/pdns_janitor/actions/workflows/docker-build-publish.yaml)

This component is responsible for wiping the pdns recursor cache when a cluster backend network ip address changes, so dns clients are always served up-to-date records when services are restarted or relocated in the cluster.

It is used by the system/svc/dns service, deployed from https://github.com/opensvc/opensvc_templates/tree/main/dns.

Tagged versions are build as 3, 3.x and 3.x.y docker images and pushed to https://hub.docker.com/r/opensvc/pdns_janitor/tags.

Note: 

* ` <3` docker image tags work with opensvc v2 agents (not build from this repository).
* `>=3` docker image tags work with opensvc v3 agents.
