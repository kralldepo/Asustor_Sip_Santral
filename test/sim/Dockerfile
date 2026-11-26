# Test double for the end-to-end test: one Asterisk that acts as the SIP
# provider (accounts line1/line2) and as phone 101 registering to SIP Santral.
FROM alpine:edge
RUN apk add --no-cache asterisk && rm -rf /etc/asterisk/*
COPY etc/ /etc/asterisk/
CMD ["/usr/sbin/asterisk", "-f", "-n", "-C", "/etc/asterisk/asterisk.conf"]
