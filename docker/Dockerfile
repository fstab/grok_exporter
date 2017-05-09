FROM ubuntu:16.04

ADD patterns /patterns
ADD grok_exporter /grok_exporter

VOLUME /config.yml

EXPOSE 9144

CMD /grok_exporter -config /config.yml