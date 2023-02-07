# pdns_janitor

This component is responsible for wiping the pdns recursor cache when a backend ip address changes, so dns clients are always served up-to-date records when services are restarted or relocated in the cluster.

It used by the system/svc/dns deployed from https://github.com/opensvc/opensvc_templates/tree/main/dns.

Note:

* opensvc/pdns_janitor <3 docker images work with opensvc v2 agents.
* opensvc/pdns_janitor >=3 docker images work with opensvc v3 agents.
