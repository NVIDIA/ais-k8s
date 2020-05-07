{{ define "aisloader_control_config.sh" }}
#
# Repeatedly sourced by control logic below
#
runid="{{ .Values.controller.runid }}"
nodecount="{{ .Values.controller.nodecount }}"
{{end}}
{{ define "aisloader_control_logic.sh" }}
#
# Control script for aisloader execution.
#
# This runs in a control pod. A DaemonSet is created and as instances start they
# first wait for us to create the 'ctlpresent' key in Redis, then they set
# their state to INIT in key 'state_${node}'' and add ${node} to the 'clientset' set.
# We wait for all daemons to check in (if we know expected number) or until 'clientset'
# stops growing for a minute or two, then set the state to RUN (or ABORT if required).
#
# If we ask for abort we expect the client to go to state ABORTED and remain there.
#
# Otherwise, once the client sees RUN it will switch state to RUNNING and execute the
# aisloader run with the configured arguments. Once the run completes the client sets
# state to one of DONE or FAILED, with run output in key 'output_${node}'
# We wait for all daemons to complete.
#

if [[ -z "$REDISHOST" ]]; then
    echo "REDISHOST not set"
    exit 1
fi

# outputs will go here
# XXX make this a volume
RESULTDIR=/tmp/results
mkdir $RESULTDIR

for ((;;)); do
    echo "Try to announce ourselves in Redis ..."
    sleep 1
    res=$(redis-cli -h $REDISHOST INCR ctlpresent)
    [[ -n "$res" ]] && break
done

#
# Wait until we have enough daemons started and registered to run the
# requested number.
#
function assemble_daemons {
    typeset -i nrequired=$1
    typeset -i ndaemon=0

    for ((;;)); do
        sleep 5
        n=$(redis-cli -h $REDISHOST SCARD clientset)
        echo "$n daemons have registered, $nrequired required"
        [[ $n -ge $nrequired ]] && break
    done

    #
    # Copy those we'll use into a working set for this run; sort
    # to always utilize same set of client nodes for a given run
    # size, where possible.
    #
    redis-cli -h $REDISHOST SORT clientset ALPHA LIMIT 0 ${nrequired} STORE benchlist
}

function reap_one {
    typeset n=$1
    typeset tag=$2

    redis-cli -h $REDISHOST GET output_$n > "$RESULTDIR/$tag/$n.out"
    redis-cli -h $REDISHOST DEL output_$n
    redis-cli -h $REDISHOST LREM benchlist 1 $n
    redis-cli -h $REDISHOST SREM clientset $n
    redis-cli -h $REDISHOST SET state_$n REAPED
}

function run_bench {
    typeset tag=$1
    typeset -i n_done=0
    typeset -i n_failed=0
    typeset -i n_weird=0
    typeset -i n_aborted=0
    typeset -i targettotal=$(redis-cli -h $REDISHOST LLEN benchlist)

    echo "$(date) Run begins for tag $tag for $targettotal nodes"

    # start each just once, any attempted repeats indicate client restart
    declare -A started

    for ((;;)); do
        # reload every iteration because we modify the set below
        declare -a nodes=( $(redis-cli -h $REDISHOST LRANGE benchlist 0 -1) )
        typeset -i n_transition=0
        typeset -i n_running=0

        for n in ${nodes[@]}; do
            case $(redis-cli -h $REDISHOST GET state_$n) in
            INIT)       if [[ -z "${started[$n]}" ]]; then
                            redis-cli -h $REDISHOST SET state_$n RUN
                            started[$n]=1
                            n_transition=$((n_transition + 1))
                            echo "$n INIT->RUN"
                        else
                            redis-cli -h $REDISHOST LREM benchlist 1 $n
                            redis-cli -h $REDISHOST SREM clientset $n
                            redis-cli -h $REDISHOST SET state_$n ABORT
                            n_aborted=$((naborted + 1))
                            echo "$n INIT -> RUN -> INIT -> ABORT (pod restarted?)"
                        fi
                        ;;
            RUN)        n_transition=$((n_transition + 1))
                        ;;
            RUNNING)    n_running=$((n_running + 1))
                        ;;
            FAILED)     n_failed+$((n_failed + 1))
                        echo "$n FAILED"
                        reap_one $n $tag
                        ;;
            DONE)       n_done=$((n_done + 1))
                        echo "$n DONE"
                        reap_one $n $tag
                        ;;
            *)          n_weird=$((n_weird + 1))
                        reap_one $n $tag
                        ;;
            esac
        done

        rstr="$(date) $n_running running, $n_done done, $n_failed failed, $n_transition transition, $n_aborted aborted, $n_weird weird"
        echo $rstr

        # Not checking for anyone stuck in transition etc
        if [[ $((n_done + n_failed + n_weird + n_aborted)) -eq $targettotal ]]; then
            echo "All have concluded"
            break
        fi

        sleep 30
    done

    echo "$(date) Run ends for tag $tag for $targettotal nodes"

    cat > "$RESULTDIR/$tag/summary" <<-EOM
    echo "STATUS SUMMARY: $rstr"
    echo "RESULT DIRECTORY: $RESULTDIR/$tag"
EOM

    cat "$RESULTDIR/$tag/summary"
}

# file derived from configmap, watch it for updates
cfile=/var/aisloader_scripts/aisloader_control_config.sh
lastmod=0

for ((;;)); do
    mod=$(stat -L --format="%Y" $cfile)
    if [[ $mod == $lastmod ]]; then
        sleep 5
        continue
    fi

    lastmod=$mod

    source $cfile

    if [[ -z "$runid" || $runid == nil ]]; then
        echo "$(date) runid seen as $runid - nothing to see here"
        sleep 30
        continue
    fi

    if [[ "$(redis-cli -h $REDISHOST SISMEMBER runids "$runid" 2>/dev/null)" == "1" ]]; then
        echo "$(date) config file changed, but runid $runid already processed"
        sleep 30
        continue
    fi

    if [[ -e "$RESULTDIR/$runid" ]]; then
        echo "Subdir $runid already exists in results dir"
        sleep 30
        continue
    fi

    # runid must be a path component
    mkdir -p "$RESULTDIR/$runid"
    if [[ $? -ne 0 ]]; then
        echo "Runid $tag must be suitable as path component"
        sleep 30
        continue
    fi

    echo "New runid $runid requested"
    redis-cli -h $REDISHOST SADD  runids "$runid"

    cp /var/aisloader_scripts/aisloader_client_config.sh "$RESULTDIR/$runid/config"

    # populate benchlist, waiting for enough daemons to register
    assemble_daemons $nodecount

    # fire in the hole!
    run_bench $runid
done

{{end}}