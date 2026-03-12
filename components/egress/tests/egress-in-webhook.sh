#!/bin/bash

docker run -d --name egress \
  --rm \
  --cap-add=NET_ADMIN \
  --sysctl net.ipv6.conf.all.disable_ipv6=1 \
  --sysctl net.ipv6.conf.default.disable_ipv6=1 \
  -e OPENSANDBOX_EGRESS_MODE=dns+nft \
  -e OPENSANDBOX_EGRESS_DENY_WEBHOOK=http://<webhook.svc>:8000 \
  -e OPENSANDBOX_EGRESS_SANDBOX_ID=mytest \
  -p 18080:18080 \
  "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/egress:latest"


sleep 5
curl -sSf -XPOST "http://127.0.0.1:18080/policy" \
  -d '{"defaultAction":"allow","egress":[{"action":"deny","target":"*.github.com"},{"action":"deny","target":"10.0.0.0/8"}]}'