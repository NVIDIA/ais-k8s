#!/usr/bin/env bash

# Copyright (c) 2018, Oracle and/or its affiliates.
# The Universal Permissive License (UPL), Version 1.0
#
# Oracle OCI Virtual Cloud Networks IP configuration script
#
# 2017-10-24 initial release
# 2017-11-21 filter out VLANs if VM
# 2017-11-21 inhibit namespaces for ubuntu 16
# 2018-02-12 fix sshd typo in help
# 2018-02-20 update copyright notice
# 2018-03-21 fix to add src routing on primary VNIC when not using namespaces
# 2018-04-05 add note on multiple VCNs
# 2018-05-10 fix to delete config by deleting default route before deleting table, added -v option
# 2018-05-22 fix to ignore interfaces enslaved to a master device
# 2018-09-14 allow namespaces for Ubuntu 16 since problems with DHCP are fixed, and inhibit for all OL/Centos 6
# 2018-09-19 fix os name and version setting for centos 6 images
# 2018-09-19 fix ip addr parsing to remove MAC VLAN interfaces
# 2018-10-25 fix secondary private ip src routing
# 2018-10-31 fix misc routing setup, fix multiple ifaces on same subnet
# 2019-11-13 removed VLAN metadata validation to work around an issue with VM metadata population for secondary VNICs

declare -r THIS=$(basename "$0")
declare -r MD_URL='http://169.254.169.254/opc/v1/vnics/'
declare -r NA='-'
declare -r RTS_FILE='/etc/iproute2/rt_tables'
declare -ir RT_ID_MIN=10 # in case lower ones are reserved
declare -ir RT_ID_MAX=255
declare -r RT_FORMAT_BM='ort${nic}vl${vltag}'
declare -r RT_FORMAT_VM='ort${nic}'
declare -r DEF_NS_FORMAT_BM='ons${nic}vl${vltag}'
declare -r DEF_NS_FORMAT_VM='ons${nic}'
declare -r MACVLAN_FORMAT='${iface}.${vltag}' # note awk script looks for this (max 15 chars)
declare -r VLAN_FORMAT='${iface}v${vltag}' # (max 15 chars)
declare -ir MTU=9000
declare -r ADD='ADD'
declare -r DELETE='DELETE'
declare -r YES='YES'
declare -r CURL=$(which curl)
declare -r IP=$(which ip)
declare -r SSHD=$(which sshd)
declare -r MODPROBE=$(which modprobe)
declare -r OS_RELEASE='/etc/os-release'
if [ -f "$OS_RELEASE" ]; then
    declare -r OS_ID=$(grep -ws ID $OS_RELEASE | cut -f 2 -d '=' | tr -d '"' | tr '[:upper:]' '[:lower:]')
    declare -r OS_VERSION=$(grep -ws VERSION_ID $OS_RELEASE | cut -f 2 -d '=' | tr -d '"')
else
    declare -r OS_RELEASE_alt='/etc/redhat-release'
    if [ -f "$OS_RELEASE_alt" ]; then
        declare -r OS_ID=$(cat $OS_RELEASE_alt|cut -f 1 -d ' ' | tr '[:upper:]' '[:lower:]')
        declare -r OS_VERSION=$(cat $OS_RELEASE_alt | cut -f 3 -d ' ')
    fi
fi
if [ -n "$OS_VERSION" ]; then
    declare -r OS_MAJ_VERSION=$(echo $OS_VERSION | cut -f 1 -d '.')
fi
declare -r SYS_CLASS_NET='/sys/class/net'
declare -A VIRTUAL_IFACES
declare IS_VM=''
declare -a MACS # all (unique) macs
declare -A MD_I_BY_MAC # index into arrays by MAC
declare -a MD_MACS
declare -a MD_ADDRS
declare -a MD_VLTAGS
declare -a MD_SCIDRS
declare -a MD_SPREFIXS
declare -a MD_SBITSS
declare -a MD_VIRTRTS
declare -a MD_VNICS
declare -a MD_NIC_IS # not set at all if vm
declare -a MD_CONFIGS # $ADD if vnic added
declare -A DUP_ADDRS # hash of addrs that appear more than once
declare -A DUP_SADDRS # hash of subnet addrs that appear more than once
# items use $NA to mean null
declare -A IP_I_BY_MAC # index into arrays by MAC
declare -a IP_MACS # runs of dups if sec addrs
declare -a IP_NSS
declare -a IP_IFACES
declare -a IP_ADDRS
declare -a IP_SADDRS
declare -a IP_SBITSS
declare -a IP_VIRTRTS
declare -a IP_STATES
declare -a IP_VLANS
declare -a IP_VLTAGS # vltag (0 if phys iface)
declare -a IP_SECADS # set to $YES if secondary addr
declare -a IP_SRCS # set to $YES if src hint
declare -a IP_NIC_IS # nic index of iface
declare -a IP_CONFIGS # $DELETE if vnic deleted
declare -a NIC_IP_IS # index of physical iface for nic index
declare -A NIC_I_BY_PHYS_IP_I # nic index for physical iface ip_i
# be sure to clear any IP_ arrays above in the read function
# options:
declare QUIET=''
declare DEBUG=''
declare START_SSHD=''
declare USE_NS=''
declare NS_FORMAT=''
declare -a SEC_ADDRS
declare -a SEC_VNICS

declare -r IFACE_AWK_SCRIPT='/tmp/oci_vcn_iface.awk'
cat >$IFACE_AWK_SCRIPT <<'EOF'
function prtiface(mac, iface, addr, sbits, state, vlan, vltag, secad) {
    if (iface != "") printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", mac, iface, addr, sbits, state, vlan, vltag, secad
}
BEGIN { iface = ""; addr = "-" }
/^[0-9]/ {
    # if not the first and prev was not a mac vlan then print previous (note print at end too)
    if (addr == "-") prtiface(mac, iface, addr, sbits, state, vlan, vltag, secad)
    addr = "-"
    sbits = "-"
    state = "-"
    macvlan = "-"
    vlan = "-"
    vltag = "-"
    secad = "-"
    if ($0 ~ /BROADCAST/ && $0 !~ /UNKNOWN/ && $0 !~ /NO-CARRIER/ && $0 !~ /master /) {
        i = index($2, "@")
        if (1 < i) {
            j = index($2, ".")
            if (j < i) { # mac vlan (not used, no addrs)
                macvlan = substr($2, 1, i - 1)
                iface = substr($2, i + 1, length($2) - i - 1) # skip : at end
                addr = "" # skip the mac vlan iface
            } else { # vlan
                vlan = substr($2, 1, i - 1)
                # extract iface/vltag from macvlan
                iface = substr($2, i + 1, j - i - 1)
                vltag = substr($2, j + 1, length($2) - j - 1) # skip : at end
            }
        } else {
            i = index($2, ":")
            if (i <= 1) { print "cannot find interface name"; exit 1 }
            iface = substr($2, 1, i - 1)
        }
        if ($0 ~ /LOWER_UP/) state = "UP"
        else state = "DOWN"
    } else iface = ""
    next
}
/ link\/ether / { mac = tolower($2) }
/ inet [0-9]/ {
    i = index($2, "/")
    if (i <= 1) { print "cannot find interface inet address"; exit 1 }
    if (addr != "-") secad = "YES"
    addr = substr($2, 0, i - 1)
    sbits = substr($2, i + 1, length($2) - i)
    prtiface(mac, iface, addr, sbits, state, vlan, vltag, secad)
}
END { if (addr == "-") prtiface(mac, iface, addr, sbits, state, vlan, vltag, secad) }
EOF

oci_vcn_err() {
    echo "Error: $1" >&2
    exit 1
}

oci_vcn_warn() {
    echo "Warning: $1" >&2
}

oci_vcn_info() {
    [ -n "$QUIET" ] || echo "Info: $1" >&2
}

oci_vcn_debug() {
    [ -z "$DEBUG" ] || echo "Debug: $1" >&2
}

oci_vcn_virtual_ifaces_read() {
    VIRTUAL_IFACES=()
    local iface
    for iface in $(ls $SYS_CLASS_NET); do
        if ls -l $SYS_CLASS_NET/$iface | grep -wq virtual; then
            VIRTUAL_IFACES[$iface]='t'
        fi
    done
}

oci_vcn_md_read() {
    # sets all MD data arrays and their index I_BY_MAC
    local -r tmpfile=$(mktemp /tmp/oci_vcn_md.XXXXX)

    # MD notes:
    # vnic order: primary first, then time created (and therefore vltag/nic)
    # BM notes: (1) may be interleaved wrt nic index (i.e. all nic 0 not guaranteed before all nic 1)
    # (2) nicIndex was supported starting around 8/23/17, but will not appear on previously
    # launched instances unless refreshed by a vnic attach or detach after that date

    # parse: force json fields on separate lines
    # WARNING: assumes no string values with commas or double quotes
    # WARNING: assumes no sub-objects with identical field names
    [ -n "$CURL" ] || oci_vcn_err "cannot find curl command"
    $CURL -s $MD_URL | tr , '\n' >"$tmpfile" || oci_vcn_err "cannot read metadata"
    MD_MACS=($(grep -w macAddr "$tmpfile" | cut -f 4 -d '"')) || exit $? # string
    local -i i
    for i in $(seq 0 $((${#MD_MACS[@]} - 1))); do
        MD_MACS[$i]="${MD_MACS[$i],,}"
    done
    MD_ADDRS=($(grep -w privateIp "$tmpfile" | cut -f 4 -d '"')) # string
    MD_VLTAGS=($(grep -w vlanTag "$tmpfile" | cut -f 2 -d ':' | tr -d ' ')) # integer
    MD_VIRTRTS=($(grep -w virtualRouterIp "$tmpfile" | cut -f 4 -d '"')) # string
    local s
    for s in $(grep -w subnetCidrBlock "$tmpfile" | cut -f 4 -d '"'); do # string
        MD_SCIDRS+=(${s})
        MD_SPREFIXS+=(${s%/*})
        MD_SBITSS+=(${s#*/})
    done
    MD_VNICS=($(grep -w vnicId "$tmpfile" | cut -f 4 -d '"'))
    MD_NIC_IS=($(grep -w nicIndex "$tmpfile" | cut -f 2 -d ':' | tr -d ' '))

    # do some validity checks on md data
    [ ${#MD_MACS[@]} -eq ${#MD_ADDRS[@]} ] || oci_vcn_err "invalid metadata: MAC or IP addresses are missing"
    # The VLAN Check here is disabled to work around an issue with VLAN metadata population for VMs
    # [ ${#MD_MACS[@]} -eq ${#MD_VLTAGS[@]} ] || oci_vcn_err "invalid metadata: MAC or VLAN tags are missing"
    [ ${#MD_MACS[@]} -eq ${#MD_VIRTRTS[@]} ] || oci_vcn_err "invalid metadata: MAC or virtual router addresses are missing"
    [ ${#MD_MACS[@]} -eq ${#MD_SPREFIXS[@]} ] || oci_vcn_err "invalid metadata: MAC or subnets are missing"
    [ ${#MD_MACS[@]} -eq ${#MD_VNICS[@]} ] || oci_vcn_err "invalid metadata: MAC or VNIC ids are missing"
    for i in $(seq 0 $((${#MD_MACS[@]} - 1))); do
        [[ ${MD_ADDRS[$i]} =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || oci_vcn_err "invalid metadata: address IP format incorrect: ${MD_ADDRS[$i]}"
        # The VLAN field validation here is disabled to work around an issue with VLAN metadata population for VMs
        # [[ ${MD_VLTAGS[$i]} =~ ^[0-9]+$ ]] || oci_vcn_err "invalid metadata: VLAN tag incorrect: ${MD_VLTAGS[$i]}"
        [[ ${MD_VIRTRTS[$i]} =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || oci_vcn_err "invalid metadata: virtual router address format incorrect: ${MD_VIRTRTS[$i]}"
        [[ ${MD_SPREFIXS[$i]} =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || oci_vcn_err "invalid metadata: subnet prefix format incorrect: ${MD_SPREFIXS[$i]}"
    done

    # set vm flag based on existence of nic index (see override in oci_vcn_ip_read)
    # get the virtual interfaces if VM
    if [ ${#MD_NIC_IS[@]} -eq 0 ]; then
        IS_VM='t'
        oci_vcn_virtual_ifaces_read
    fi

    # create reverse lookup
    for i in "${!MD_MACS[@]}"; do
        MD_I_BY_MAC[${MD_MACS[$i]}]=$i
    done
    rm "$tmpfile"

    # find the duplicate addrs, if any
    local -A addrs=()
    local addr
    for addr in "${MD_ADDRS[@]}"; do
        if [ -n "${addrs[$addr]}" ]; then DUP_ADDRS[$addr]='t'; fi
        addrs[$addr]='t'
    done

    # find the duplicate subnet addrs, if any
    local -A saddrs=()
    for addr in "${MD_SPREFIXS[@]}"; do
        if [ -n "${saddrs[$addr]}" ]; then DUP_SADDRS[addr]='t'; fi
        saddrs[addr]='t'
    done
}

oci_vcn_ip_route_table_name() {
    local -ir nic=$1 # format looks for "${nic}", note this is really $nic_i
    local -ir vltag=$2
    local format="$RT_FORMAT_VM"

    [ -n "$IS_VM" ] || format="$RT_FORMAT_BM"
    eval echo "$format"
}

oci_vcn_ip_route_table_name_ip_i() {
    # use only when ip already setup
    local -ir ip_i=$1
    local -ir nic_i=${IP_NIC_IS[$ip_i]}
    local -ir vltag=${IP_VLTAGS[$ip_i]}

    oci_vcn_ip_route_table_name $nic_i $vltag
}

oci_vcn_ip_route_table_exists() {
    local -r rt_name=$1
    if grep -qsw $rt_name $RTS_FILE; then
        echo "$rt_name"
    fi
}

oci_vcn_ip_route_table_find_unused_id() {
    # read all the current route table id/name pairs
    # mapfile will create array with each line an element
    local lines
    mapfile -t lines < <(cat $RTS_FILE | grep -E '^[0-9]' | tr '\t' ' ' | tr -s ' ' ' ') || oci_vcn_err "cannot read route tables file $RTS_FILE"
    local line
    local -A rt_by_id
    for line in "${lines[@]}"; do
        local -a pair=($line)
        local id=${pair[0]}
        local name=${pair[1]}
        rt_by_id[$id]=$name
    done

    # find first id not used
    local -i unused=-1
    local -i i
    for i in $(seq $RT_ID_MIN $RT_ID_MAX); do
        if [ -z "${rt_by_id[$i]}" ]; then
            unused=$i
            break
        fi
    done
    [ $unused -ne -1 ] ||  oci_vcn_err "cannot find unused id in route tables file $RTS_FILE"
    echo $unused
}

oci_vcn_ip_route_table_create() {
    local -ir nic_i=$1
    local -ir vltag=$2
    local -r skip_if_exists=$3
    local -r rt_name=$(oci_vcn_ip_route_table_name $nic_i $vltag)

    # Check if the route table exists
    local rt_exists
    rt_exists=$(oci_vcn_ip_route_table_exists $rt_name) || exit $?
    if [ -n "$rt_exists" ]; then # already exists
        if [ -n "$skip_if_exists" ]; then
            oci_vcn_debug "create route table $rt_name already exists, skipping"
            return
        fi
        oci_vcn_warn "route table $rt_name already exists, reusing"
    else # create
        local -i rt_id
        rt_id=$(oci_vcn_ip_route_table_find_unused_id) || exit $?
        oci_vcn_debug "create route table $rt_name ($rt_id)"
        echo "$rt_id    $rt_name" >> $RTS_FILE
    fi
    echo "$rt_name"
}

oci_vcn_ip_route_table_del() {
    local -r rt_name=$1
    local -r tmpfile=$(mktemp /tmp/oci_vcn_rt_tables.XXXXX)

    if ! grep -qsw $rt_name $RTS_FILE; then
        oci_vcn_debug "rt $rt_name not in $RTS_FILE"
        return
    fi
    oci_vcn_debug "rt $rt_name delete"
    cp -p $RTS_FILE "$tmpfile" # in case grep fails
    grep -vw $rt_name $RTS_FILE > "$tmpfile"
    mv "$tmpfile" $RTS_FILE
    echo 't'
}

oci_vcn_ip_routing_add() {
    local -ir md_i=$1
    local -ir nic_i=$2
    local -r iface=$3
    local -r ns=$4
    local -r skip_if_exists=$5
    local -ir vltag="${MD_VLTAGS[$md_i]}"
    local -r addr="${MD_ADDRS[$md_i]}"
    local -r sprefix="${MD_SPREFIXS[$md_i]}"
    local -r virtrt="${MD_VIRTRTS[$md_i]}"
    local -r scidr="${MD_SCIDRS[$md_i]}"

    # set a default route to the gateway/virtual router
    if [ -n "$ns" ]; then
        local -r nscmd="netns exec $ns $IP"
        # using a namespace: iface is assumed only one in namespace, so just set a
        # default route to the gateway in the main route table
        oci_vcn_debug "default route add"
        $IP $nscmd route add default via $virtrt || oci_vcn_err "cannot add namespace $ns default gateway $virtrt"
        oci_vcn_info "added namespace $ns default route to gateway $virtrt"
    else
        # the default route (in main route table) already defined through primary vnic.
        # this adds a rule to lookup a route table for any packets sourced from addr
        # and then the route table has the default route for that addr.
        # this allows packets from protocol services that reply with src addr
        # set to route back out through the iface (prevents asymmetric routing).

        # check for dup addrs and subnet addrs
        if [ -n "${DUP_ADDRS[$addr]}" ]; then
            oci_vcn_warn "IP address $addr is a duplicate, skipping creating source route rule"
            return
        fi
        [ -z "${DUP_SADDRS[$sprefix]}" ] || oci_vcn_warn "duplicate subnet prefix $sprefix"

        # create route table and add a default route via the gateway, and subnet route
        local rt_name
        rt_name=$(oci_vcn_ip_route_table_create $nic_i $vltag $skip_if_exists) || exit $?
        [ -n "$rt_name" ] || return
        oci_vcn_debug "route table add rules"
        $IP route add default via $virtrt dev $iface table $rt_name || oci_vcn_err "cannot add default route via $virtrt on $iface to table $rt_name"
        $IP route add $scidr dev $iface table $rt_name || oci_vcn_err "cannot add subnet $scidr route on $iface to table $rt_name"

        # create source-based rule to use table
        oci_vcn_debug "src rule add"
        $IP rule add from $addr lookup $rt_name || oci_vcn_err "cannot add rule from $addr use table $rt_name"
        oci_vcn_info "added rule for routing from $addr lookup $rt_name with default via $virtrt"
    fi
}

oci_vcn_ip_routing_del() {
    local -ir ip_i=$1
    local -r ns="${IP_NSS[$ip_i]#$NA}"
    local -r iface="${IP_IFACES[$ip_i]}"

    # for namespaces the subnet and default routes will be auto deleted with the namespace
    if [ -z "$ns" ]; then
        # delete rule
        local -r rt_name=$(oci_vcn_ip_route_table_name_ip_i $ip_i)
        local had_rule=''
        if $IP rule | grep -qsw $rt_name; then # rule(s) exists
            # note that there may be secondary private ips rules using the same table
            oci_vcn_debug "rules to table $rt_name delete"
            while $IP rule del lookup $rt_name 2>/dev/null; do true; done
            had_rule='t'
        fi

        # delete route table
        local had_table
        had_table=$(oci_vcn_ip_route_table_del $rt_name) || exit $?
        ([ -z "$had_rule" ] && [ -z "$had_table" ]) || oci_vcn_info "removed routing on interface $iface"
    fi
}

oci_vcn_ip_routing_sec_addr_add() {
    local -ir iface_ip_i=$1
    local -r addr=$2
    local -r ns="${IP_NSS[$iface_ip_i]#$NA}"

    # need to add source rule so reuse the routing table from the primary private ip
    # not needed if in namespace
    # not needed if on pri iface (uses main route table for dest and default route)
    if [ -z "$ns" ] && [ $iface_ip_i -ne 0 ]; then
        # create source-based rule to use iface's routing table
        local -r rt_name=$(oci_vcn_ip_route_table_name_ip_i $iface_ip_i)
        oci_vcn_debug "sec addr src rule add"
        $IP rule add from $addr lookup $rt_name || oci_vcn_err "cannot add rule from secondary $addr use table $rt_name"
        oci_vcn_info "added rule for routing from secondary IP address $addr lookup $rt_name"
    fi
}

oci_vcn_ip_routing_sec_addr_del() {
    local -ir ip_i=$1
    local -r addr=$2
    local -r ns="${IP_NSS[$ip_i]#$NA}"

    # deconfig the routing (see comments in add)
    if [ -z "$ns" ] && [ $ip_i -ne 0 ]; then
        $IP rule del from $addr 2>/dev/null # ok if already deleted
        oci_vcn_info "deleted rule for routing from secondary IP address $addr"
    fi
}

oci_vcn_ip_routes_read() {
    #ip r list 192.168.1.0/24
    local -ir ip_i=$1
    local -r ns="${IP_NSS[$ip_i]#$NA}"
    local -r iface="${IP_IFACES[$ip_i]}"
    local nscmd=''
    local virtrt="$NA"

    if [ -n "$ns" ]; then
        nscmd="netns exec $ns $IP"
    else
        # no namespace: check for route table
        local -r rt_name=$(oci_vcn_ip_route_table_name_ip_i $ip_i)
        local rt_exists
        rt_exists=$(oci_vcn_ip_route_table_exists $rt_name) || exit $?
        if [ -n "$rt_exists" ]; then # exists
            # check for rule
            if $IP rule | grep -qsw $rt_name; then # rule exists that uses route table
                # look for default route in table, note table may exist but be empty
                # "default via 10.0.0.1 dev ens3"
                local -a def_entry
                def_entry=($($IP route show table $rt_name | grep -sw ^default))
                if [ -n "${def_entry[2]}" ]; then
                    virtrt="${def_entry[2]}"
                    oci_vcn_debug "default route: $virtrt"
                else # emtpy table: delete rule and table
                    virtrt="$NA"
                    oci_vcn_ip_routing_del $ip_i
                fi
            else # clean up route table since no rule uses it
                $(oci_vcn_ip_route_table_del $rt_name)
            fi
        fi
    fi

    # read the routes
    local sprefix="$NA"
    local sbits="$NA"
    local src="$NA"
    # mapfile will create array with each line an element
    mapfile -t routes < <($IP $nscmd route | grep -w $iface) || oci_vcn_err "cannot read IP routes for interface $iface"
    local line
    for line in "${routes[@]}"; do
        local -a route=($line)
        if [ "${route[0]}" = 'default' ]; then
            # "default via 10.0.0.1 dev ens3"
            virtrt="${route[2]}"
            oci_vcn_debug "default route via: $virtrt"
        elif [ "${route[0]#169.}" = "${route[0]}" ]; then # not cavium route
            # "10.0.0.0/24 dev ens3 proto kernel scope link src 10.0.0.2"
            local -i i
            for i in $(seq 0 $((${#route[@]} - 1))); do
                local x="${route[$i]}"
                if [ $i -eq 0 ]; then
                    sprefix=${x%/*}
                    sbits=${x#*/}
                    if [ "$sprefix" = "$sbits" ]; then # not valid line
                        sprefix="$NA"
                        sbits="$NA"
                        break
                    else
                        oci_vcn_debug "subnet route: $sprefix/$sbits"
                    fi
                elif [ "$x" = 'src' ]; then src="$YES"
                fi
            done
        fi
    done
    IP_VIRTRTS[$ip_i]="$virtrt"
    IP_SADDRS[$ip_i]="$sprefix"
    IP_SBITSS[$ip_i]="$sbits"
    IP_SRCS[$ip_i]="$src"
}

oci_vcn_macvlan_name() {
    local -r iface=$1
    local -r vltag=$2
    eval echo "$MACVLAN_FORMAT"
}

oci_vcn_vlan_name() {
    local -r iface=$1
    local -r vltag=$2
    eval echo "$VLAN_FORMAT"
}

oci_vcn_ip_ns_name() {
    local -ir nic=$1 # format looks for "${nic}"
    local -ir vltag=$2

    if [ -n "$USE_NS" ] && [ -z "$NS_FORMAT" ]; then
        if [ -n "$IS_VM" ]; then NS_FORMAT="$DEF_NS_FORMAT_VM"
        else NS_FORMAT="$DEF_NS_FORMAT_BM"; fi
    fi
    eval echo "$NS_FORMAT"
}

oci_vcn_ip_ns_svcs_stop() {
    local -r ns=$1
    local pids
    pids=$($IP netns pids $ns) || oci_vcn_err "cannot get ids for processes in namespace $ns"
    if [ -n "$pids" ]; then
        kill -TERM $pids || oci_vcn_err "cannot terminate namespace $ns processes: $pids"
        oci_vcn_info "terminated namespace $ns processes: $pids"
    fi
}

oci_vcn_ip_ns_svcs_start() {
    local -r ns=$1
    if [ -n "$START_SSHD" ]; then # start SSH daemon
        $IP netns exec $ns $SSHD || oci_vcn_err "cannot start ssh daemon"
        oci_vcn_info "started namespace $ns ssh daemon"
    fi
}

oci_vcn_ip_ns_del() {
    local -r ns=$1
    # note also deletes vlans and routes
    $IP netns del $ns || oci_vcn_err "cannot delete namespace $ns"
    oci_vcn_info "deleted namespace $ns"
}

oci_vcn_ip_ns_create() {
    local -ir nic_i=$1
    local -ir vltag=$2

    [ -n "$MODPROBE" ] || oci_vcn_err "cannot find modprobe command"
    $MODPROBE 8021q || oci_vcn_err "failed to load 8021q module"
    local ns
    ns=$(oci_vcn_ip_ns_name $nic_i $vltag) || exit $?
    $IP netns add $ns || oci_vcn_err "cannot create namespace $ns"
    oci_vcn_info "created namespace $ns"
    echo "$ns"
}

oci_vcn_ip_addr_add_iface() {
    local -ir md_i=$1
    local -ir ip_i=$2 # index of physical iface/nic
    local -r ns=$3
    local iface="${IP_IFACES[$ip_i]}"
    local -r physns="${IP_NSS[$ip_i]#$NA}"
    local -r mac="${MD_MACS[$md_i]}"
    local -ir vltag="${MD_VLTAGS[$md_i]}"
    local -r addr="${MD_ADDRS[$md_i]}"
    local -r sbits="${MD_SBITSS[$md_i]}"
    local vlan=''

    # must be adding to physical iface/nic
    [ -z "${IP_VLANS[$ip_i]#$NA}" ] || oci_vcn_err "cannot add IP address $addr to virtual interface ${IP_VLANS[$ip_i]}"

    # create virtual interface if needed (bm cases)
    local macvlan=''
    if [ -z "$IS_VM" ] && [ $vltag -ne 0 ]; then
        # bm vnics need a virtual iface except for vltag=0
        # if physical iface/nic is in a namespace we must go there to create
        local physnscmd=''
        if [ -n "$physns" ]; then
            physnscmd="netns exec $physns $IP"
        fi

        # create a mac vlan from physical iface/nic
        oci_vcn_debug "macvlan link add $macvlan"
        macvlan=$(oci_vcn_macvlan_name $iface $vltag) || exit $?
        $IP $physnscmd link add link $iface name $macvlan address $mac type macvlan \
            || oci_vcn_err "cannot create MAC VLAN interface $macvlan for MAC address $mac"

        # if physical iface/nic is in a namespace pull out the created mac vlan
        if [ -n "$physns" ]; then
            $IP $physnscmd link set $macvlan netns 1
        fi

        # create an ip vlan on top of the mac vlan
        oci_vcn_debug "vlan link add"
        vlan=$(oci_vcn_vlan_name $iface $vltag) || exit $?
        $IP link add link $macvlan name $vlan type vlan id $vltag \
            || oci_vcn_err "cannot create VLAN $vlan on MAC VLAN $macvlan"
    fi

    # use namespace, if option
    local nscmd=''
    local -r dev="${vlan:-$iface}"
    if [ -n "$ns" ]; then
        nscmd="netns exec $ns $IP"
        # move the iface(s) to the target namespace
        if [ -n "$macvlan" ]; then
            oci_vcn_debug "macvlan link move $ns"
            $IP link set dev $macvlan netns $ns || oci_vcn_err "cannot move MAC VLAN $macvlan into namespace $ns"
        fi
        oci_vcn_debug "$dev link move $ns"
        $IP link set dev $dev netns $ns || oci_vcn_err "cannot move interface $dev into namespace $ns"
    fi

    # add IP address to iface:
    # the netmask is specified to create a route in the main route table
    # if this iface is on the same subnet as another iface then the route will not have an effect
    # (until the other iface is removed)
    oci_vcn_debug "addr $addr/$sbits add on $dev ns '$ns'"
    $IP $nscmd addr add $addr/$sbits dev $dev || oci_vcn_err "cannot add IP address $addr/$sbits on interface $dev"

    if [ -n "$macvlan" ]; then # set vlans up
        oci_vcn_debug "vlans set up"
        $IP $nscmd link set dev $macvlan mtu $MTU up || oci_vcn_err "cannot set MAC VLAN $macvlan up"
        $IP $nscmd link set dev $vlan mtu $MTU up || oci_vcn_err "cannot set VLAN $vlan up"
    else
        oci_vcn_debug "$iface set up"
        $IP $nscmd link set dev $iface mtu $MTU up || oci_vcn_err "cannot set interface $iface MTU"
    fi

    oci_vcn_info "added IP address $addr on interface $dev with MTU $MTU"
    echo "$dev"
}

oci_vcn_ip_addr_del_iface() {
    local -ir ip_i=$1
    local -r iface="${IP_IFACES[$ip_i]}"
    local -r ns="${IP_NSS[$ip_i]#$NA}"
    local -r vlan="${IP_VLANS[$ip_i]#$NA}"
    local -r secad="${IP_SECADS[$ip_i]#$NA}"
    local nscmd=''

    if [ -n "$ns" ]; then
        nscmd="netns exec $ns $IP"
    fi
    if [ "$secad" != "$YES" ] && [ -n "$vlan" ]; then
        # delete vlan and macvlan, removes the addrs (pri and sec) as well
        oci_vcn_debug "$ns link delete"
        local -ir vltag="${IP_VLTAGS[$ip_i]}"
        local macvlan
        macvlan=$(oci_vcn_macvlan_name $iface $vltag) || exit $?
        $IP $nscmd link del link $vlan dev $macvlan || oci_vcn_err "cannot remove VLAN $vlan"
        oci_vcn_info "removed VLAN $vlan"
    else
        # delete addr from phys iface
        # deleting namespace will move phys iface back to main
        # note that we may be deleting sec addr from a vlan here
        local -r addr="${IP_ADDRS[$ip_i]#$NA}"
        local -r dev="${vlan:-$iface}"
        local bits="${IP_SBITSS[$ip_i]#$NA}"

        [ "$secad" != "$YES" ] || bits=32
        oci_vcn_debug "addr $addr del ns '$ns' dev $dev"
        $IP $nscmd addr del $addr/$bits dev $dev || oci_vcn_err "cannot remove IP address $addr/$bits from interface $dev"
        oci_vcn_info "removed IP address $addr from interface $dev"
    fi
}

oci_vcn_ip_addr_add() {
    local -ir md_i=$1
    local -r mac="${MD_MACS[$md_i]}"
    local -r addr="${MD_ADDRS[$md_i]}"
    local ns=''
    local iface=''
    local -i nic_i
    local -i vltag

    # note that when adding an addr to a physical iface ip_i will be the index of the
    # addr, but when creating a vlan for addr ip_i will not be the vlan iface but its phys one
    local -i ip_i

    if [ -z "$IS_VM" ]; then
        # bm vnics' physical ifaces are identified by nic index
        nic_i=${MD_NIC_IS[$md_i]}
        [ $nic_i -lt ${#NIC_IP_IS[@]} ] || oci_vcn_err "cannot find interface for NIC $nic_i"
        ip_i=${NIC_IP_IS[$nic_i]}
        iface="${IP_IFACES[$ip_i]}"
        vltag=${MD_VLTAGS[$md_i]}
    else
        # vm vnics' physical ifaces are identified by matching mac
        local found=''
        ip_i=0
        local ip_mac
        for ip_mac in "${IP_MACS[@]}"; do
            if [ "$ip_mac" = "$mac" ]; then
                found='t'
                break
            fi
            ip_i+=1
        done
        [ -n "$found" ] || oci_vcn_err "cannot find interface matching VNIC MAC $mac"
        iface="${IP_IFACES[$ip_i]}"
        nic_i=${IP_NIC_IS[$ip_i]}
        vltag=0
    fi

    # check that there is no current addr on iface
    if [ -n "$IS_VM" ] || [ "${MD_VLTAGS[$md_i]}" = "0" ]; then # putting addr directly on iface
        local -r ip_addr="${IP_ADDRS[$ip_i]#$NA}"
        [ -z "$ip_addr" ] || oci_vcn_err "IP address $ip_addr already added on interface $iface"
    fi

    # make sure physical iface/nic is up
    if [ "${IP_STATES[$ip_i]}" != "UP" ]; then
        $IP link set dev $iface up || oci_vcn_err "cannot set interface $iface up"
    fi

    # create namespace if requested
    local ns=''
    if [ -n "$USE_NS" ]; then
        ns=$(oci_vcn_ip_ns_create $nic_i $vltag) || exit $?
        # if working on a physical iface we need to set its namespace so that any
        # vlan created subsequently will know how to find it
        [ $vltag -ne 0 ] || IP_NSS[$ip_i]="$ns"
    fi

    # add addr to iface (possibly creating vlan)
    local dev
    dev=$(oci_vcn_ip_addr_add_iface $md_i $ip_i $ns) || exit $?

    # setup routes
    oci_vcn_ip_routing_add $md_i $nic_i $dev $ns

    # if namespace then wait for changes and start services
    if [ -n "$ns" ]; then
        sleep 1 # namespace changes seem to take time
        oci_vcn_ip_ns_svcs_start $ns
    fi
}

oci_vcn_ip_sec_addr_add() {
    local -ir iface_ip_i=$1
    local -r addr=$2
    local -r iface="${IP_IFACES[$iface_ip_i]}"
    local -r vlan="${IP_VLANS[$iface_ip_i]#$NA}"
    local -r dev="${vlan:-$iface}"
    local -r ns="${IP_NSS[$iface_ip_i]#$NA}"
    local nscmd=''
    local nsinfo=''

    # config routing
    oci_vcn_ip_routing_sec_addr_add $iface_ip_i $addr

    # config the addr on the iface
    if [ -n "$ns" ]; then
        nscmd="netns exec $ns $IP"
        nsinfo=" in namespace $ns"
    fi
    oci_vcn_info "adding secondary IP address $addr to interface (or VLAN) $dev$nsinfo"
    $IP $nscmd addr add $addr/32 dev $dev || oci_vcn_err "cannot add secondary IP address $addr on interface $dev$nsinfo"
}

oci_vcn_ip_sec_addr_del() {
    local -ir ip_i=$1
    local -r deconfig_all=$2
    local -r addr=${IP_ADDRS[$ip_i]}
    local -r iface=${IP_IFACES[$ip_i]}
    local -r vlan="${IP_VLANS[$ip_i]#$NA}"
    local -r dev="${vlan:-$iface}"
    local -r ns="${IP_NSS[$ip_i]#$NA}"
    local nscmd=''
    local nsinfo=''

    [ "${IP_SECADS[$ip_i]}" = "$YES" ] || oci_vcn_err "not a secondary IP address: $addr"

    # remove the addr on the iface (see comments in add)
    # no need if deconfig all and on a vlan or in namespace
    if [ -z "$deconfig_all" ] || ([ -z "$vlan" ] && [ -z "$ns" ]); then
        if [ -z "$deconfig_all" ] && [ -n "$ns" ]; then
            nscmd="netns exec $ns $IP"
            nsinfo=" in namespace $ns"
        fi

        oci_vcn_info "removing secondary IP address $addr from interface (or VLAN) $dev$nsinfo"
        $IP $nscmd addr del $addr/32 dev $dev || oci_vcn_err "cannot remove IP address $addr on interface $dev"
    fi

    # remove the routing (see comments in add)
    oci_vcn_ip_routing_sec_addr_del $ip_i $addr
}

oci_vcn_sec_addr_is_provisioned() {
    local -r find_addr=$1
    local -r find_vnic=$2
    local -i i
    local found=''

    for i in $(seq 0 $((${#SEC_ADDRS[@]} - 1))); do
        local addr=${SEC_ADDRS[$i]}
        local vnic=${SEC_VNICS[$i]}
        if [ "$find_addr" = "$addr" ] && [ "$find_vnic" = "$vnic" ]; then
            found='t'
            break
        fi
    done
    echo "$found"
}

oci_vcn_ip_addr_del() {
    local -ir ip_i=$1
    local -r ns="${IP_NSS[$ip_i]#$NA}"
    local -r secad="${IP_SECADS[$ip_i]#$NA}"

    [ $ip_i -ne 0 ] || oci_vcn_err "cannot remove primary VNIC"

    if [ "$secad" != "$YES" ]; then
        if [ -n "$ns" ]; then
            # stop services in namespace
            oci_vcn_ip_ns_svcs_stop $ns
        fi

        # remove routing
        oci_vcn_ip_routing_del $ip_i
    fi

    # remove addr
    oci_vcn_ip_addr_del_iface $ip_i

    if [ "$secad" != "$YES" ] && [ -n "$ns" ]; then
        # delete namespace
        oci_vcn_ip_ns_del $ns
        sleep 1 # namespace changes seem to take time
    fi
}

oci_vcn_ip_ifaces_read() {
    local -r ns="$1"
    local nscmd=''
    local -a iface_datas

    if [ -n "$ns" ]; then # change ip command to use namespace
        nscmd="netns exec $ns $IP"
    fi

    # read the interfaces in namespace (if any)
    # mapfile will create array with each line an element
    mapfile -t iface_datas < <($IP $nscmd addr show | awk -f $IFACE_AWK_SCRIPT) || oci_vcn_err "cannot read IP addresses"
    if [ ${#iface_datas[@]} -eq 0 ]; then
        # if reading physical ifaces, must be at least 1
        [ -n "$ns" ] || oci_vcn_err "cannot locate interfaces"
        # empty namespace: probably result of a VM VNIC delete
        # note empty namespaces do not survive reboot
        $IP netns del $ns || oci_vcn_err "cannot delete empty namespace $ns"
        oci_vcn_warn "deleted empty namespace $ns"
    else
        local -r nsna="${ns:-$NA}"
        local -i ip_i=${#IP_MACS[@]} # continue from previous ns (if any)
        local line
        for line in "${iface_datas[@]}"; do
            # line items are in order printed by awk script print
            # note that $NA is used to mean null (i.e. not set)
            oci_vcn_debug "iface line: $line"
            local -a iface_data=($line)
            local iface="${iface_data[1]}"
            # filter out virtual interfaces if VM (assume created by user for other purpose)
            if { [ -z "$IS_VM" ] || [ -z "${VIRTUAL_IFACES[$iface]}" ]; } && [ "${iface_data[1]}" != "cni0" ]; then
                local mac="${iface_data[0]}"
                IP_MACS+=("$mac")
                IP_NSS+=("$nsna")
                IP_IFACES+=("$iface")
                IP_ADDRS+=("${iface_data[2]}")
                IP_SBITSS+=("${iface_data[3]}")
                IP_STATES+=("${iface_data[4]}")
                IP_VLANS+=("${iface_data[5]}")
                IP_VLTAGS+=("${iface_data[6]}")
                local secad="${iface_data[7]}"
                IP_SECADS+=("$secad")
                [ "$secad" = "$YES" ] || IP_I_BY_MAC["$mac"]=$ip_i # primary addrs only
                ip_i+=1
            fi
        done
    fi
}

oci_vcn_ip_read() {
    IP_I_BY_MAC=()
    IP_MACS=()
    IP_NSS=()
    IP_IFACES=()
    IP_ADDRS=()
    IP_SADDRS=()
    IP_SBITSS=()
    IP_VIRTRTS=()
    IP_STATES=()
    IP_VLANS=()
    IP_VLTAGS=()
    IP_SECADS=()
    IP_SRCS=()
    IP_NIC_IS=()
    NIC_IP_IS=()
    NIC_I_BY_PHYS_IP_I=()

    # read the non-namespace ifaces and any addrs on them
    oci_vcn_ip_ifaces_read
    # read ifaces in all the namespaces (if any), note some os's don't support netns
    mapfile -t nss < <($IP netns 2>/dev/null)
    local ns
    # for ns in "${nss[@]}"; do
    #     oci_vcn_ip_ifaces_read $ns
    # done

    # set vltag 0 (for phys ifaces) and read os routes for all ifaces
    local -i ip_i
    for ip_i in $(seq 0 $((${#IP_MACS[@]} - 1))); do
        # any iface w/o vltag has tag 0
        [ -n "${IP_VLTAGS[$ip_i]#$NA}" ] || IP_VLTAGS[$ip_i]=0
        oci_vcn_ip_routes_read $ip_i
    done

    # find the "physical" iface indices: vltag 0 and not secondary addr
    # note vm ifaces are considered physical
    # these may not be in nic index order due to inclusion in namespaces (see next)
    local -r tmpfile=$(mktemp /tmp/oci_vcn_ifaces.XXXXX)
    local -A ip_i_by_phys_iface
    for ip_i in $(seq 0 $((${#IP_MACS[@]} - 1))); do
        if [ ${IP_VLTAGS[$ip_i]} -eq 0 ] && [ "${IP_SECADS[$ip_i]}" != "$YES" ]; then
            # physical iface/nic
            local iface=${IP_IFACES[$ip_i]}
            ip_i_by_phys_iface[$iface]=$ip_i
            echo "$iface" >> "$tmpfile"
        fi
        NIC_I_BY_PHYS_IP_I[$ip_i]=-1
    done

    # sort physical ifaces by first number (either at end or in middle) in iface name
    # this will provide the nic index
    local -i nic_i=0
    local iface
    for iface in $(cat "$tmpfile" | awk -- '{ match($1, /[0-9]+/); print substr($1, RSTART, RLENGTH), $1 }' | sort -n | cut -f 2 -d ' '); do
        local ip_i=${ip_i_by_phys_iface[$iface]}
        NIC_IP_IS+=($ip_i)
        NIC_I_BY_PHYS_IP_I[$ip_i]=$nic_i
        nic_i+=1
    done
    rm "$tmpfile"

    # for each iface (phys or vlan) get nic index
    for ip_i in $(seq 0 $((${#IP_MACS[@]} - 1))); do
        local iface=${IP_IFACES[$ip_i]}
        local -i phys_ip_i=${ip_i_by_phys_iface[$iface]}
        IP_NIC_IS[$ip_i]=${NIC_I_BY_PHYS_IP_I[$phys_ip_i]}
    done

    # fix up missing nic index for bms (see also oci_vcn_md_read): bms will be missing nic index metadata
    # if they were created before and have not had a vnic attached or detached since 8/23/17.
    # if there is a secondary vnic created and configured (before 8/23/17) we can tell it is a bm
    # because there will either be configured vlans or more vnics than physical ifaces.
    # note that gen 2 shapes are post 8/23/17 and, hence, will have nic indices already set.
    # a missing nic index for an old bm will not matter if there are no vnics to configure.
    if [ ${#MD_NIC_IS[@]} -eq 0 ] && [ ${#NIC_IP_IS[@]} -eq 1 ] && \
        ([ ${#IP_MACS[@]} -gt 1 ] || [ ${#MD_MACS[@]} -gt 1 ]); then # configured or new secondaries
        local -i md_i
        for md_i in $(seq 0 $((${#MD_MACS[@]} - 1))); do
            MD_NIC_IS+=(0)
        done
        oci_vcn_info "legacy BM instance detected"
        IS_VM=''
    fi
}

oci_vcn_read() {
    # assumes md info is already read
    # reads ip configs and creates single array of all macs
    # (fixes vm interfaces if random mac)
    [ -n "$IP" ] || oci_vcn_err "cannot find ip command"
    local warn_ifaces=''
    local -i attempt
    for attempt in 1 2; do
        # initialize md/ip shared arrays
        MACS=("${MD_MACS[@]}")
        # read all of ip config info and see if it matches md info
        oci_vcn_ip_read
        MD_CONFIGS=()
        local -i md_i=0
        local mac
        for mac in "${MD_MACS[@]}"; do
            MD_CONFIGS[$md_i]="$NA"
            local -i ip_i=${IP_I_BY_MAC[$mac]:--1}
            if [ $ip_i -lt 0 ]; then
                # if there is no corresponding iface mac: add
                # note ifaces with random macs will be detected below and this will be retried
                MD_CONFIGS[$md_i]="$ADD"
            else
                local addr="${IP_ADDRS[$ip_i]#$NA}"
                if [ -z "$addr" ]; then
                    # matching mac iface does not have address: add
                    # make sure it is not a (corrupted) vlan that had been configured previously
                    [ ${IP_VLTAGS[$ip_i]} -eq 0 ] || oci_vcn_err "VLAN (with MAC $mac) configured but missing IP address (must manually fix)"
                    MD_CONFIGS[$md_i]="$ADD"
                fi
            fi
            md_i+=1
        done
        local -i ip_i=0
        IP_CONFIGS=()
        local retry=''
        warn_ifaces=''
        local -A new_macs=() # for deduping secondary addr macs
        for mac in "${IP_MACS[@]}"; do
            IP_CONFIGS[$ip_i]="$NA"
            local addr="${IP_ADDRS[$ip_i]#$NA}"
            # note that the primary vnic will be matched up (permanently)
            local -i md_i=${MD_I_BY_MAC[$mac]:--1}
            if [ $md_i -lt 0 ]; then
                # no metadata mac corresponding to ip mac
                local iface="${IP_IFACES[$ip_i]}"
                if [ -n "$addr" ]; then
                    # addr is configured: should be deleted
                    # bm case (in vm case ifaces are auto-deleted when vnic is detached)
                    IP_CONFIGS[$ip_i]="$DELETE"
                elif [ -z "$IS_VM" ]; then
                    # bm iface mac w/o addr and w/o md mac:
                    # skip if phys iface, else addr deleted w/o deleting vlan?
                    if [ ${IP_VLTAGS[$ip_i]} -ne 0 ]; then
                        IP_CONFIGS[$ip_i]="$DELETE"
                        warn_ifaces="$warn_ifaces $iface"
                    fi
                else
                    # vm iface mac w/o addr and w/o md mac: assume random mac
                    ([ $attempt -eq 1 ] && [ "${IP_STATES[$ip_i]}" = 'DOWN' ]) || oci_vcn_err "interface $iface with MAC $mac does not have corresponding metadata"
                    # assume vm case 1st since less likely addr was deleted
                    # attempt mac fix by turning iface up, then retry
                    # this is probably an Intel driver problem
                    # could look at /sys/class/net/<iface>/addr_assign_type
                    $IP link set dev $iface up || oci_vcn_err "cannot set interface $iface up"
                    retry='t'
                fi
                if [ -z "${new_macs[$mac]}" ]; then
                    new_macs[$mac]='t'
                    MACS+=("$mac") # accumulate all unique macs
                fi
                # TODO else validate for consistency?: addr, vltag, subnet, virtrt
            elif [ "${IP_SECADS[$ip_i]}" = "$YES" ]; then
                local is_prov=$(oci_vcn_sec_addr_is_provisioned $addr "${MD_VNICS[$md_i]}")
                if [ -z "$is_prov" ]; then
                    IP_CONFIGS[$ip_i]="$DELETE"
                fi
            fi
            ip_i+=1
        done
        [ -n "$retry" ] || break
    done

    if [ -n "$warn_ifaces" ]; then
        oci_vcn_warn "no VNIC (or MAC does not match) and no address, interfaces will be marked for delete:$warn_ifaces"
    fi
}

oci_vcn_config_or_deconfig_sec_addrs() {
    local -r do_config="$1" # config if not empty, else deconfig
    local found=''
    # vnics must be configured, whether config or deconfig secondary addrs
    local -i i
    for i in $(seq 0 $((${#SEC_ADDRS[@]} - 1))); do
        local addr=${SEC_ADDRS[$i]}
        local vnic=${SEC_VNICS[$i]}
        # find vnic's mac
        local mac=''
        local -i md_i
        for md_i in $(seq 0 $((${#MD_MACS[@]} - 1))); do
            if [ "$vnic" = "${MD_VNICS[$md_i]}" ]; then
                mac="${MD_MACS[$md_i]}"
                break
            fi
        done
        [ -n "$mac" ] || oci_vcn_err "cannot find VNIC for secondary IP address $addr on $vnic"
        # find mac in ip config
        local -i iface_ip_i=-1
        local already_config=''
        local -i ip_i
        for ip_i in $(seq 0 $((${#IP_MACS[@]} - 1))); do
            if [ "$mac" = "${IP_MACS[$ip_i]}" ]; then # put on this iface if not already configured
                [ $iface_ip_i -ge 0 ] || iface_ip_i=$ip_i # 1st is interface (secondary addrs come after)
                if [ "$addr" = "${IP_ADDRS[$ip_i]}" ]; then # already configured
                    already_config='t'
                    break
                fi
            fi
        done
        [ $iface_ip_i -ge 0 ] || oci_vcn_err "cannot find interface for secondary IP address $addr on $vnic"
        if [ -n "$do_config" ] && [ -z "$already_config" ]; then # configure
            oci_vcn_ip_sec_addr_add $iface_ip_i $addr
            found='t'
        elif [ -z "$do_config" ] && [ -n "$already_config" ]; then # deconfigure
            # note this path is only if deconfiguring just the secondaries
            oci_vcn_ip_sec_addr_del $ip_i
            found='t'
        fi
    done
    echo "$found"
}

oci_vcn_config() {
    local found=''
    local mac
    local -A configed

    # fix up config: md will indicate adds, ip deletes
    local -i md_i
    for md_i in $(seq 0 $((${#MD_CONFIGS[@]} - 1))); do
        local config="${MD_CONFIGS[$md_i]#$NA}"
        if [ "$config" = "$ADD" ]; then
            oci_vcn_info "adding IP config for VNIC MAC ${MD_MACS[$md_i]} with id ${MD_VNICS[$md_i]}"
            oci_vcn_ip_addr_add $md_i
            found='t'
        fi
    done
    local del_vmac=''
    local -i ip_i
    for ip_i in $(seq 0 $((${#IP_CONFIGS[@]} - 1))); do
        local config="${IP_CONFIGS[$ip_i]#$NA}"
        if [ "$config" = "$DELETE" ]; then
            # del all pri addrs and sec addrs (unless its vlan is being deleted)
            local mac="${IP_MACS[$ip_i]}"
            local secad="${IP_SECADS[$ip_i]#$NA}"
            if [ "$secad" != "$YES" ] || [ "$del_vmac" != "$mac" ]; then
                local addr="${IP_ADDRS[$ip_i]}"
                oci_vcn_info "removing IP config of address $addr from MAC $mac"
                oci_vcn_ip_addr_del $ip_i
                found='t'
                # keep track of last deleted vlan
                [ "${IP_VLTAGS[$ip_i]}" -eq 0 ] || del_vmac=$mac
            fi
        fi
    done

    # config secondary addrs, if any
    local sec_addrs_found=''
    if [ ${#SEC_ADDRS[@]} -gt 0 ]; then
        # reread config if there were changes so that secondaries are put in the correct place
        if [ -n "$found" ]; then
            sleep 2 # wait for newly created ifaces to settle
            oci_vcn_read
        fi
        sec_addrs_found=$(oci_vcn_config_or_deconfig_sec_addrs 't') || exit $?
    fi

    [ -n "$found" ] || [ -n "$sec_addrs_found" ] || oci_vcn_info "no changes, IP configuration is up-to-date"
}

oci_vcn_deconfig_all() {
    local -i ip_i=0
    local found=''
    local mac
    for mac in "${IP_MACS[@]}"; do
        local addr="${IP_ADDRS[$ip_i]#$NA}"
        if [ -n "$addr" ]; then # ip is configured
            # note that primaries are encountered first
            if [ "${IP_SECADS[$ip_i]}" != "$YES" ]; then # primary addr
                if [ $ip_i -gt 0 ]; then # skip pri vnic, pri addr
                    local -i md_i=${MD_I_BY_MAC[$mac]:--1}
                    local missing=" missing"
                    local vnicmsg=''
                    if [ $md_i -ge 0 ]; then vnicmsg=" with id ${MD_VNICS[$md_i]}"; missing=""; fi
                    oci_vcn_info "removing IP config of address $addr for$missing VNIC MAC $mac$vnicmsg"
                    oci_vcn_ip_addr_del $ip_i
                    found='t'
                fi
            else # secondary addr
                oci_vcn_ip_sec_addr_del $ip_i 't'
                found='t'
            fi
        fi
        ip_i+=1
    done
    if [ -z "$found" ]; then
        oci_vcn_info "no changes, no IP configuration to delete"
    fi
}

oci_vcn_show() {
    local -r fmt="%-6s %-15s %-15s %-5s %-15s %-10s %-3s %-10s %-5s %-11s %-5s %-17s %s\n"
    printf "$fmt" CONFIG ADDR SPREFIX SBITS VIRTRT NS IND IFACE VLTAG VLAN STATE MAC VNIC
    local mac
    for mac in "${MACS[@]}"; do # all known macs
        local config="$NA"
        local addr="$NA"
        local nic_i="$NA"
        local vltag="$NA"
        local sprefix="$NA"
        local sbits="$NA"
        local virtrt="$NA"
        local ns="$NA"
        local iface="$NA"
        local vlan="$NA"
        local state="$NA"
        local vnic="$NA"
        local -i md_i=${MD_I_BY_MAC[$mac]:--1}
        if [ $md_i -ge 0 ]; then # in md: ADD, or no change depending on ip info
            config="${MD_CONFIGS[$md_i]}"
            nic_i="${MD_NIC_IS[$md_i]:-$NA}"
            addr="${MD_ADDRS[$md_i]}"
            [ -n "$IS_VM" ] || vltag="${MD_VLTAGS[$md_i]}" # not used in vms
            sprefix="${MD_SPREFIXS[$md_i]}"
            sbits="${MD_SBITSS[$md_i]}"
            virtrt="${MD_VIRTRTS[$md_i]}"
            vnic="${MD_VNICS[$md_i]}"
        fi
        # find the ip info on this mac, note that there could be none, one,
        # or multiple addrs if secondaries addrs exist (they will come after primary)
        local -i ip_i=${IP_I_BY_MAC[$mac]:--1} # index of primary addr, if any
        if [ $ip_i -ge 0 ]; then
            local -i pri_ip_i=$ip_i
            local secad=''
            while true; do
                secad="${IP_SECADS[$ip_i]#$NA}"
                [ $pri_ip_i -eq $ip_i ] || [ -n "$secad" ] || break
                vlan="${IP_VLANS[$ip_i]:-$NA}"
                iface="${IP_IFACES[$ip_i]}"
                ns="${IP_NSS[$ip_i]}"
                state="${IP_STATES[$ip_i]}"
                local cfg="${IP_CONFIGS[$ip_i]#$NA}"
                [ -z "$cfg" ] || config="$cfg"
                if [ $md_i -lt 0 ]; then # not in md, fill with ip info
                    addr="${IP_ADDRS[$ip_i]}"
                    sbits="${IP_SBITSS[$ip_i]}"
                    virtrt="${IP_VIRTRTS[$ip_i]}"
                    [ -n "$IS_VM" ] || vltag="${IP_VLTAGS[$ip_i]}"
                elif [ -n "$secad" ]; then
                    addr="${IP_ADDRS[$ip_i]}"
                fi
                local -i nic_phys=${NIC_I_BY_PHYS_IP_I[$ip_i]}
                [ -n "$secad" ] || [ $nic_phys -lt 0 ] || nic_i=$nic_phys # don't show if sec addr
                if [ -z "$secad" ]; then
                    printf "$fmt" "$config" "$addr" "$sprefix" "$sbits" "$virtrt" "$ns" "$nic_i" "$iface" "$vltag" "$vlan" "$state" "$mac" "$vnic"
                else
                    printf "$fmt" "$config" "$addr" "$NA" "$NA" "$NA" "$NA" "$NA" "$NA" "$NA" "$NA" "$NA" "$NA" "$NA"
                fi
                ip_i+=1
            done
        else
            printf "$fmt" "$config" "$addr" "$sprefix" "$sbits" "$virtrt" "$ns" "$nic_i" "$iface" "$vltag" "$vlan" "$state" "$mac" "$vnic"
        fi
    done
}

oci_vcn_help() {
    cat <<EOF
NAME
    $THIS -- display and configure Oracle OCI Virtual Cloud Networks on instance

SYNOPSIS
    $THIS [-s] [-e <IP address> <VNIC OCID>]
    $THIS -c [-q] [-s] [-n [<format>] [-r]] [-e <IP address> <VNIC OCID> [-e ...]]
    $THIS -d [-q] [-s] [-e <IP address> <VNIC OCID>]

DESCRIPTION
    This shows the current OCI Virtual interface Cards provisioned in the cloud
    and configured on this instance. When a secondary VNIC is provisioned in OCI it must
    be explicitly configured on the instance using this script or similar commands.

    The first version of this command displays the currently provisioned VNICs and the
    current IP configuration for this instance. VNICs that are not yet configured are
    marked with '$ADD' and IP configurations that no longer have an associated VNIC
    are marked with '$DELETE'.

    The second version, with -c, configures VNICs that do not have an IP configuration
    and deletes the IP configurations of VNICs that are not currently provisioned.
    This puts the instance IP configuration in sync with current OCI provisioning.
    If one or more optional -e options are present the secondary IP addresses are
    configured on the same interfaces as the corresponding VNIC.

    The configuring interfaces can optionally be placed inside separate network
    namespaces. This is necessary when VNICs are in subnets (different VCNs) with
    overlapping address blocks and the network applications are not bound directly
    to interfaces. Network namespaces require applications to be launched in them
    explicitly (via 'ip netns exec <ns>') in order to establish association with
    the interface. When namespaces are not used, policy-based routing is configured
    to provide a default route to the secondary VNIC\'s virtual router (default
    gateway) when the VNIC\'s address is the source address.

    Bare Metal secondary VNICs are configured using VLANs (where there is no
    corresponding physical interface). These will look like 2 addition interfaces
    when showing IP links, with names like '$MACVLAN_FORMAT' for the MAC VLAN
    and '$VLAN_FORMAT' for the IP VLAN.

    The third version, -d, deletes all IP configuration for provisioned secondary VNICs
    as long as there is no -e option. If one or more optional -e options are present
    only the given secondary IP addresses are deconfigured and the remaining configuration
    is left as is.

    This script is made to be run periodically to pick up changes in VNIC provisioning
    (whether adding or deleting). Note that these IP configuration changes are not
    persistent, the script must, at a minimum, be run on each startup.

    -c          Add IP configuration for VNICs that are not configured and delete
                for VNICs that are no longer provisioned.
    -d          Deconfigure all VNICs (except the primary). If a -e option is also
                present only the secondary IP address(es) are deconfigured.
    -e <IP address> <VNIC OCID>
                Secondary private IP address to configure or deconfigure.
    -h          Print help.
    -n [<format>]
                When configuring, place interfaces in namespace identified by the given
                format. Format can include \$nic and \$vltag variables. The name
                defaults to '$DEF_NS_FORMAT_BM' for BMs and '$DEF_NS_FORMAT_VM' for VMs.
                When configuring multiple VNICs ensure the namespaces are unique.
    -q          Suppress information messages.
    -r          Start sshd in namespace (if -n is present)
    -s          Show information on all provisioning and interface configuration.
                This is the default action if no options are given.
                Columns:
                    CONFIG  '$ADD' indicates missing IP config, '$DELETE' missing VNIC
                    ADDR    IP address
                    SPREFIX subnet CIDR prefix
                    SBITS   subnet mask bits
                    VIRTRT  virutal router IP address
                    NS      namespace (if any)
                    IND     interface index (if BM)
                    IFACE   interface (underlying physical if VLAN is also set)
                    VLTAG   VLAN tag (if BM)
                    VLAN    IP virtual LAN (if any)
                    STATE   state of interface
                    MAC     MAC address
                    VNIC    VNIC object identifier
    -v          Verbose information messages.

EXAMPLES
    $THIS
        Show all provisioned VNICs and configured IP addresses.
    $THIS -c
        Set configuration without a namespace.
    $THIS -c -n ''
        Set configuration using a namespace with the default format.
    $THIS -c -n 'myns\$vltag'
        Set configuration using a namespace with format 'myns\$vltag'.

SEE ALSO:
    OCI networking overview including route tables and security lists:
        https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/overview.htm
    Note: secondary VNIC in different VCN not handled, requires manually adding default route:
        ip route add <VCN2 CIDR> via <VCN2 sec VNIC subnet virt router ip> dev <VCN2 sec VNIC ifname>
EOF
}

# TODO secondary private IPs in metadata

declare show=''
declare config=''
declare deconfig=''
declare os_ver="$OS_ID-$OS_VERSION"
declare os_maj_ver="$OS_ID-$OS_MAJ_VERSION"
while [ $# -ge 1 ]; do
    declare opt="$1"
    shift
    case $opt in
        -c) config='t';;
        -d) deconfig='t';;
        -e) if [ $# -lt 2 ]; then oci_vcn_err "secondary private IP address option requires <IP address> <VNIC OCID>"; fi
            SEC_ADDRS+=($1); shift
            SEC_VNICS+=($1); shift
            ;;
        -n) if [ "$os_maj_ver" = "ol-6" ] || [ "$os_maj_ver" = "centos-6" ]; then
                oci_vcn_err "namespaces not supported on this os version ($os_ver)"
            fi
            if [ $# -ge 1 ] && ! [[ "$1" =~ ^\- ]]; then
                [ -z "$1" ] || NS_FORMAT="$1"
                shift
            fi
            USE_NS='t';;
        -r) START_SSHD='t'
            [ -n "$SSHD" ] || oci_vcn_err "missing sshd command";;
        -s) show='t';;
        -h) oci_vcn_help; exit 0;;
        -q) if [ -n "$DEBUG" ]; then oci_vcn_err "cannot specify quiet with verbose"; fi
            QUIET='t';;
        -v) if [ -n "$QUIET" ]; then oci_vcn_err "cannot specify verbose with quiet"; fi
            DEBUG='t';;
        -*) oci_vcn_err "unknown option $opt";;
    esac
done

[ -z "$START_SSHD" ] || [ -n "$USE_NS" ] || oci_vcn_err "cannot start sshd if namespace is not created"

[ $EUID -eq 0 ] || oci_vcn_err "must be run as root"

# read all metadata and ip config
oci_vcn_md_read
oci_vcn_read

# process options
if [ -n "$config" ]; then
    [ -z "$deconfig" ] || oci_vcn_err "conflicting options"
    oci_vcn_config
    [ -z "$show" ] || oci_vcn_read # reread if show
elif [ -n "$deconfig" ]; then
    if [ ${#SEC_ADDRS[@]} -gt 0 ]; then # just deconfig addrs
        oci_vcn_config_or_deconfig_sec_addrs
    else # deconfig all
        oci_vcn_deconfig_all
    fi
    [ -z "$show" ] || oci_vcn_read # reread if show
else
    show='t'
fi
[ -z "$show" ] || oci_vcn_show
