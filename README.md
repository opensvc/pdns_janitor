# pdns_janitor

This component is responsible for wiping the pdns recursor cache when a cluster backend network ip address changes, so dns clients are always served up-to-date records when services are restarted or relocated in the cluster.

It is used by the system/svc/dns service, deployed from https://github.com/opensvc/opensvc_templates/tree/main/dns.

Note on the opensvc/pdns_janitor docker image tags:

* ` <3` tags work with opensvc v2 agents.
* `>=3` tags work with opensvc v3 agents.
