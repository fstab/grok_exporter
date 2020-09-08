package tailer

import (
	ctx "context"
	"sync"

	"github.com/Shopify/sarama"
	configuration "github.com/fstab/grok_exporter/config/v3"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/sirupsen/logrus"
)

type KafkaTailer struct {
	lines  chan *fswatcher.Line
	errors chan fswatcher.Error
}

type consumer struct {
	ready     chan bool
	lineChan  chan *fswatcher.Line
	errorChan chan fswatcher.Error
}

func (t KafkaTailer) Lines() chan *fswatcher.Line {
	return t.lines
}

func (t KafkaTailer) Errors() chan fswatcher.Error {
	return t.errors
}

func (t KafkaTailer) Close() {
	logrus.Info("Close method called")

}

// RunKafkaTailer runs the kafka tailer
func RunKafkaTailer(cfg *configuration.InputConfig) fswatcher.FileTailer {
	lineChan := make(chan *fswatcher.Line)
	errorChan := make(chan fswatcher.Error)

	tailer := &KafkaTailer{
		lines:  lineChan,
		errors: errorChan,
	}

	go initKafkaConsumer(lineChan, errorChan, cfg)

	return *tailer
}

func initKafkaConsumer(lineChan chan *fswatcher.Line, errorChan chan fswatcher.Error, cfg *configuration.InputConfig) {

	version, err := sarama.ParseKafkaVersion(cfg.KafkaVersion)
	if err != nil {
		logrus.Panicf("[Kafka] Error parsing Kafka version: %v", err)
	}

	/**
	 * Construct a new Sarama configuration.
	 * The Kafka cluster version has to be defined before the consumer/producer is initialized.
	 */

	consumer := consumer{
		ready:     make(chan bool),
		lineChan:  lineChan,
		errorChan: errorChan,
	}

	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = version

	switch cfg.KafkaPartitionAssignor {
	case "sticky":
		kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategySticky
	case "roundrobin":
		kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	case "range":
		kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRange
	default:
		consumer.errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "[Kafka] Unrecognized consumer group partition assignor!")
	}

	if cfg.KafkaConsumeFromOldest {
		kafkaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	}

	/**
	 * Setup a new Sarama consumer group
	 */

	ctx, cancel := ctx.WithCancel(ctx.Background())
	client, err := sarama.NewConsumerGroup(cfg.KafkaBrokers, cfg.KafkaConsumerGroupName, kafkaConfig)
	if err != nil {
		consumer.errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "[Kafka] Error creating client")
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// `Consume` should be called inside an infinite loop, when a
			// server-side rebalance happens, the consumer session will need to be
			// recreated to get the new claims
			if err := client.Consume(ctx, cfg.KafkaTopics, &consumer); err != nil {
				consumer.errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "[Kafka] Error from consumer")
			}
			// check if context was cancelled, signaling that the consumer should stop
			if ctx.Err() != nil {
				logrus.Infof("[Kafka] Consumer %s goroutine exiting.", cfg.KafkaConsumerGroupName)
				return
			}
			consumer.ready = make(chan bool)
		}
	}()

	<-consumer.ready // Await till the consumer has been set up
	logrus.Infof("[Kafka] Consumer %s active.", cfg.KafkaConsumerGroupName)

	select {
	case <-ctx.Done():
		logrus.Info("[Kafka] Consumer terminating: context cancelled")
	}

	cancel()
	wg.Wait()

	if err = client.Close(); err != nil {
		consumer.errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "[Kafka] Error closing client")
		return
	}

	logrus.Info("[Kafka] Client has been closed")

}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	close(consumer.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (consumer *consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (consumer *consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	for message := range claim.Messages() {
		logrus.Debugf("[Kafka] Message content: %s", string(message.Value))
		session.MarkMessage(message, "")
		consumer.lineChan <- &fswatcher.Line{Line: string(message.Value)}
	}

	return nil
}
