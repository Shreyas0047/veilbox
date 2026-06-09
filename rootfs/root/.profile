export PS1='\[\e[1;31m\]root@veilbox\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)
[ -n "$IP" ] && echo "  IP: $IP"
