export PS1='\[\e[1;31m\]root@veilbox\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
IP=$(ip addr show eth0 2>/dev/null | grep 'inet ' | head -1)
IP="${IP#*inet }"
IP="${IP%%/*}"
[ -n "$IP" ] && echo "  IP: $IP"
echo "  Default container limits: 1 CPU, 512MB RAM (override with --cpus / --memory)"
