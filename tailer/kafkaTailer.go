package tailer

import (
	"encoding/json"
	"github.com/fstab/grok_exporter/config/v2"
	"github.com/optiopay/kafka"
	"log"
	"strings"
)

type kafkaTailer struct {
	lines  chan string
	errors chan error
}

func (t *kafkaTailer) Lines() chan string {
	return t.lines
}

func (t *kafkaTailer) Errors() chan error {
	return t.errors
}

func (t *kafkaTailer) Close() {
	// broker.Close()
}

func RunConsumer(broker kafka.Client, lineChan chan string, errorChan chan error, topic string, cfg *v2.Config) {
	var data map[string]interface{}
	var partition int32 = 0
	log.Printf("Creating consumer for topic %s", topic)
	conf := kafka.NewConsumerConf(topic, partition)
	conf.StartOffset = kafka.StartOffsetNewest
	consumer, err := broker.Consumer(conf)
	if err != nil {
		log.Fatalf("cannot create kafka consumer for %s:%d: %s", topic, partition, err)
		errorChan <- err
	}
	log.Printf("Consumer for topic %s is ready", topic)
	for {
		msg, err := consumer.Consume()
		if err != nil {
			if err != kafka.ErrNoData {
				log.Printf("cannot consume %q topic message: %s", topic, err)
			}
			errorChan <- err
			return
		}
		s := []string{}
		if cfg.Input.Jsonfields == "" {
			s = append(s, string(msg.Value))
		} else {
			if err := json.Unmarshal([]byte(msg.Value), &data); err != nil {
				log.Fatalf("Cannot unmarshal JSON message %s because of the error: %s", string(msg.Value), err)
			}
			for _, field := range strings.Split(cfg.Input.Jsonfields, ",") {
				s = append(s, data[field].(string))
			}
		}
		line := strings.Join(s, " ")
		if cfg.Global.Debug {
			log.Println("Sending line to parcer: " + line)
		}
		lineChan <- line
	}
}

func RunKafkaTailer(broker kafka.Client, cfg *v2.Config) Tailer {
	lineChan := make(chan string)
	errorChan := make(chan error)
	topics := strings.Split(cfg.Input.Topics, ",")
	for _, topic := range topics {
		go RunConsumer(broker, lineChan, errorChan, topic, cfg)
	}
	return &kafkaTailer{
		lines:  lineChan,
		errors: errorChan,
	}
}
