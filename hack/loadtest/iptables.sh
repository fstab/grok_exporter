#!/bin/bash

iptables -F

iptables -P INPUT ACCEPT
iptables -P FORWARD ACCEPT
iptables -P OUTPUT ACCEPT

# DNS
iptables -A INPUT -p udp --dport 53 -j ACCEPT

iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -i lo -j ACCEPT

iptables -A INPUT -p icmp --icmp-type echo-request -j ACCEPT
iptables -A INPUT -p icmp --icmp-type echo-reply -j ACCEPT

iptables -A INPUT -i eth0 -p tcp --dport 22 -j ACCEPT
iptables -A INPUT -i eth0 -p tcp --dport 9090 -j ACCEPT
iptables -A INPUT -i eth0 -p tcp --dport 3000 -j ACCEPT

iptables -A INPUT -p udp -j REJECT --reject-with icmp-port-unreachable
iptables -A INPUT -p tcp -j REJECT --reject-with tcp-reset
iptables -A INPUT -p icmp -j REJECT --reject-with icmp-proto-unreachable

iptables -P INPUT DROP
iptables -P FORWARD DROP

# IPv6

ip6tables -F INPUT
ip6tables -F FORWARD
ip6tables -F OUTPUT
ip6tables -F

ip6tables -A INPUT -p icmpv6 -j ACCEPT
ip6tables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

ip6tables -P INPUT DROP
