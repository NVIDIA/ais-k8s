{{ define "aisloader_client_config.sh" -}}
#
# Aisloader config, expanded from values.yaml into ready-to-use shell form.
#

set -x
coord=false
[[ -n "$REDISHOST" ]] && coord=true
postrun_snooze="{{ .Values.controller.postrun_snooze }}"

ais_rel={{ required "AIStore release name (.Values.ais_release) is required" .Values.ais_release | quote }}
ais_ns={{ required "AIStore release namespace (.Values.ais_namespace) is required" .Values.ais_namespace | quote }}

#
# All clients target the same AIS instance
#
endpoint="http://${ais_rel}-ais-proxy.${ais_ns}:{{ .Values.config.port.default }}"

#
# Initialize associative arrays with default arguments
#
declare -A bucket=( [_default]="{{ .Values.config.bucket.default }}" ) 
declare -A duration=( [_default]="{{ .Values.config.duration.default }}" )
declare -A pctput=( [_default]="{{ .Values.config.pctput.default }}" )
declare -A cleanup=( [_default]="{{ .Values.config.cleanup.default }}" )
declare -A readertype=( [_default]="{{ .Values.config.readertype.default }}" )
declare -A numworkers=( [_default]="{{ .Values.config.numworkers.default }}" )
declare -A minsize=( [_default]="{{ .Values.config.minsize.default }}" )
declare -A maxsize=( [_default]="{{ .Values.config.maxsize.default }}" )
declare -A seed=( [_default]="{{ .Values.config.seed.default }}" )
declare -A statsinterval=( [_default]="{{ .Values.config.statsinterval.default }}" )
declare -A uniquegets=( [_default]="{{ .Values.config.uniquegets.default }}" )
declare -A putshards=( [_default]="{{ .Values.config.putshards.default }}" )
declare -A maxputs=( [_default]={{ .Values.config.maxputs.default | quote }} )
declare -A readlen=( [_default]={{ .Values.config.readlen.default | quote }} )
declare -A readoff=( [_default]={{ .Values.config.readoff.default | quote }} )

# Node-specific values for bucket, if any
{{ range .Values.config.bucket.specific }}
bucket[{{ .node | quote }}]={{ .value | quote }}
{{ end }}

# Node-specific values for duration, if any
{{ range .Values.config.duration.specific }}
duration[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for pctput, if any
{{ range .Values.config.pctput.specific }}
pctput[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for cleanup, if any
{{ range .Values.config.cleanup.specific }}
cleanup[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for readertype, if any
{{ range .Values.config.readertype.specific }}
readertype[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for numworkers, if any
{{ range .Values.config.numworkers.specific }}
numworkers[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for minsize, if any
{{ range .Values.config.minsize.specific }}
minsize[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for maxsize, if any
{{ range .Values.config.maxsize.specific }}
maxsize[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for seed, if any
{{ range .Values.config.seed.specific }}
seed[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for statsinterval, if any
{{ range .Values.config.statsinterval.specific }}
statsinterval[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for uniquegets, if any
{{ range .Values.config.uniquegets.specific }}
uniquegets[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for putshards, if any
{{ range .Values.config.putshards.specific }}
putshards[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for maxputs, if any
{{ range .Values.config.maxputs.specific }}
maxputs[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for readlen, if any
{{ range .Values.config.readlen.specific }}
readlen=[{{ .node }}]={{ .value | quote }}
{{ end }}

# Node-specific values for readoff, if any
{{ range .Values.config.readoff.specific }}
readoff=[{{ .node }}]={{ .value | quote }}
{{ end }}

# Export environment variables
{{ range .Values.aisloaderEnv }}
export {{ .name }}={{ .value | quote }}
{{ end }}

{{end}}
{{ define "aisloader_client_logic.sh" }}
#
# Client execution logic. Separated from above so we can poll updates to the above
#

#
# Execute aisloader with the arguments as dictated by the sourced config file
#
function do_aisloader {
    n=$1
    of=$2

    bktpat=${bucket[$n]:-${bucket['_default']}}
    mybkt=${bktpat/%%s/$n}

    #
    # Interpret seed value
    #
    case "${seed[$n]:-${seed['_default']}}" in
    fromhostip)
        SEED=${MY_HOSTIP#[0-9]*\.[0-9]*\.[0-9]*\.}
        ;;
    random)
        RANDOM=$(dd if=/dev/random bs=1 count=2 2>/dev/null | od -t u2 | head -1 | awk '{print $2}')
        SEED=$RANDOM
        ;;
    0)
        SEED=0
        ;;
    *)
        SEED=$RANDOM
        ;;
    esac

    set -x
    env AIS_ENDPOINT=${endpoint} stdbuf -o0  aisloader \
        -bucket=$mybkt \
        -check-statsd=true \
        -seed=$SEED \
        -duration=${duration[$n]:-${duration['_default']}} \
        -pctput=${pctput[$n]:-${pctput['_default']}} \
        -cleanup=${cleanup[$n]:-${cleanup['_default']}} \
        -readertype=${readertype[$n]:-${readertype['_default']}} \
        -numworkers=${numworkers[$n]:-${numworkers['_default']}} \
        -minsize=${minsize[$n]:-${minsize['_default']}} \
        -maxsize=${maxsize[$n]:-${maxsize['_default']}} \
        -statsinterval=${statsinterval[$n]:-${statsinterval['_default']}} \
        -uniquegets=${uniquegets[$n]:-${uniquegets['_default']}} \
        -putshards=${putshards[$n]:-${putshards['_default']}} \
        -maxputs=${maxputs[$n]:-${maxputs['_default']}} \
        -readlen=${readlen[$n]:-${readlen['_default']}} \
        -readoff=${readoff[$n]:-${readoff['_default']}} \
        2>&1 | tee $of

    return $?
}

#
# Wrapper to do_aisloader which synchronizes with any controlling pod
#
function run_aisloader {
    n=$1

    if ! $coord; then
        echo "No redis coordination, running independently"
        do_aisloader "$n" /dev/null
        return
    fi

    #
    # Run output will be to this file and will be returned via redis
    #
    output=$(mktemp)

    #
    # Loop until Redis is up and our controlling pod indicates it is live
    #
    while [[ $(redis-cli -h $REDISHOST EXISTS ctlpresent) -ne 1 ]]; do
        echo "Waiting for controller ..."
        sleep 1
    done

    #
    # Now register our existence in the clientset after creating our control key
    #
    redis-cli -h $REDISHOST SET state_$n INIT
    redis-cli -h $REDISHOST SADD clientset $n

    #
    # Wait until controller move us beyond INIT state
    #
    while [[ $(redis-cli -h $REDISHOST GET state_$n) == "INIT" ]]; do
        echo "Waiting in INIT state ..."
        sleep 1
    done

    #
    # Controller moves us to one of RUN or ABORT
    #
    case "$(redis-cli -h $REDISHOST GET state_$n)" in
    ABORT)  echo "ABORT requested"
            redis-cli -h $REDISHOST SET state_$n ABORTED
            return
            ;;
    RUN)    echo "RUN requested"
            redis-cli -h $REDISHOST SET state_$n RUNNING
            ;;
    *)      echo "Unexpected command from controller"
            redis-cli -h $REDISHOST SET state_$n "FAILED"
            ;;
    esac

    #
    # source config now - it may have changed while we were waiting in INIT state
    # to be asked to do some work
    #
    source /var/aisloader_scripts/aisloader_client_config.sh

    touch $output
    do_aisloader $n $output
    rc=$?

    #
    # Return output before indicating final state
    #
    redis-cli -h $REDISHOST -x SET output_$n < $output

    if [[ $rc -eq 0 ]]; then
            redis-cli -h $REDISHOST SET state_$n DONE
    else
            redis-cli -h $REDISHOST SET state_$n FAILED
    fi

    #
    # Wait for REAPED state to say that the controller has grabbed our state
    # and output (and will have removed us from the clientset). Client can
    # restart and go back to INIT state.
    #
    for ((;;)); do
        sleep 5
        [[ $(redis-cli -h $REDISHOST GET state_$n) == REAPED ]] && break
        echo "Waiting for REAPED"
    done

    #
    # Stick around here (for debug) is asked. Once we return this pod will
    # recycle.
    #
    if [[ -n "$postrun_snooze" ]]; then
        echo "Post-run snooze of ${postrun_snooze}s"
        sleep $snooze
    fi
}

run_aisloader $1
{{end}}