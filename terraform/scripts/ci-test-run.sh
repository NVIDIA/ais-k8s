#!/bin/bash

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

echo "starting AIS logs"
kubectl logs -f --max-log-requests 10 -l 'component in (proxy,target)' &

admin_container=$(kubectl get pods --namespace default -l "component=admin" -o jsonpath="{.items[0].metadata.name}")

kubectl cp $CURRENT_DIR/ci-test-script.sh $admin_container:/tmp/ci-test-script.sh
kubectl exec $admin_container -- bash -c "chmod +x /tmp/ci-test-script.sh && /tmp/ci-test-script.sh"