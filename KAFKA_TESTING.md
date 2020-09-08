## How to test the Kafka integration

1. Ensure you have docker setup on your system.

2. Create the following config and save it as `docker-compose-kafka-single.yml`
```
---
version: '2'

services:
  zookeeper:
    image: confluentinc/cp-zookeeper:latest
    hostname: zookeeper
    ports:
      - 32181:32181
    environment:
      ZOOKEEPER_CLIENT_PORT: 32181
      ZOOKEEPER_TICK_TIME: 2000
    extra_hosts:
      - "moby:127.0.0.1"
      - "localhost: 127.0.0.1"

  kafka:
    image: confluentinc/cp-kafka:latest
    hostname: kafka
    ports:
      - 9092:9092
    depends_on:
      - zookeeper
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:32181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    extra_hosts:
      - "moby:127.0.0.1"
      - "localhost: 127.0.0.1"
```

3. Create the kafka cluster:
```
docker-compose -f docker-compose-kafka-single.yml up
```

4. Create the necessary topic:
```
docker-compose -f docker-compose-kafka-single.yml exec kafka kafka-topics --create --bootstrap-server localhost:9092 --replication-factor 1 --partitions 1 --topic grok_exporter_test
```

5. Publish a sample test message:
```
docker-compose -f docker-compose-kafka-single.yml exec kafka bash -c "echo 'this is a test' | kafka-console-producer --request-required-acks 1 --broker-list localhost:9092 --topic grok_exporter_test"
  ```

6. Given that the `grok_exporter` was properly configured and you're matching for a string in the message you've previously published to kafka, you should have matches that appear on the metrics page.
