// Package alilogs provides the logdriver for forwarding container logs to Ali Log Service
package alilogs

import (
	"fmt"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aliyun-fc/go-loghub"
	"github.com/docker/docker/daemon/logger"
	"github.com/golang/protobuf/proto"
)

/*
Ali logging driver usage
	docker run -d --name test-logger \
 		--log-driver alilogs \
		--log-opt alilogs-endpoint=cn-hangzhou.log.aliyuncs.com \
		--log-opt alilogs-project=test_project \
		--log-opt alilogs-logstore=test-logstore \

		// You can add these extra attributes to log message
		--log-opt labels=attr1,attr2,attr3 \
		--label attr1=attr1Value \
		--label attr2=attr2Value \
		--label attr3=attr3Value \

		// You assign these environment variables for alilogs logging driver to work
		// "securityToken" and "topic" are optional
	    --log-opt env=accessKeyID,accessKeySecret,securityToken,topic \
		--env "accessKeyID=xxx" \
		--env "accessKeySecret=xxx" \
		--env "securityToken=xxx" \
		--env "topic=demo_topic" \
		log-producer
*/

const (
	name        = "alilogs"
	endpointKey = "alilogs-endpoint"
	projectKey  = "alilogs-project"
	logstoreKey = "alilogs-logstore"
	envKey      = "env"
	labelsKey   = "labels"

	accessKeyIDEnvKey     = "accessKeyID"
	accessKeySecretEnvKey = "accessKeySecret"
	securityTokenEnvKey   = "securityToken"
	topicEnvKey           = "topic"

	// PutLogs limit in Loghub, 3MB or 4096 records per put
	batchPublishFrequency = 5 * time.Second
	maximumBytesPerPut    = 3145728
	maximumLogsPerPut     = 4096
)

type logStream struct {
	endpoint         string
	projectName      string
	logstoreName     string
	topic            string
	extraLogContents []*sls.LogContent
	client           AliLogAPI
	logGroup         *sls.LogGroup
	messages         chan *logger.Message
	lock             sync.RWMutex
	closed           bool
}

type contextParams struct {
	accessKeyID     string
	accessKeySecret string
	securityToken   string
	topicName       string
	extraContents   []*sls.LogContent
}

// init registers the alilogs driver
func init() {
	if err := logger.RegisterLogDriver(name, New); err != nil {
		logrus.Fatal(err)
	}
	if err := logger.RegisterLogOptValidator(name, ValidateLogOpt); err != nil {
		logrus.Fatal(err)
	}
}

// New creates an alilogs logger using the configuration passed in on the context
func New(ctx logger.Context) (logger.Logger, error) {
	endpoint := ctx.Config[endpointKey]
	projectName := ctx.Config[projectKey]
	logstoreName := ctx.Config[logstoreKey]

	contextInput, err := parseContext(&ctx)
	if err != nil {
		return nil, err
	}
	aliLogClient, err := NewAliLogClient(endpoint, projectName, logstoreName, contextInput.accessKeyID, contextInput.accessKeySecret, contextInput.securityToken)
	if err != nil {
		return nil, err
	}
	containerStream := &logStream{
		endpoint:         endpoint,
		projectName:      projectName,
		logstoreName:     logstoreName,
		topic:            contextInput.topicName,
		extraLogContents: contextInput.extraContents,
		client:           aliLogClient,
		logGroup: &sls.LogGroup{
			Topic: proto.String(contextInput.topicName),
			Logs:  []*sls.Log{},
		},
		messages: make(chan *logger.Message, maximumLogsPerPut),
	}

	go containerStream.collectLogs()
	return containerStream, nil
}

// Name returns the name of ali logging driver
func (ls *logStream) Name() string {
	return name
}

// Log submits messages for logging by an instance of the alilogs logging driver
func (ls *logStream) Log(msg *logger.Message) error {
	ls.lock.RLock()
	defer ls.lock.RUnlock()
	if !ls.closed {
		// buffer up the data, making sure to copy the Line data
		ls.messages <- msg
	}
	return nil
}

// Close closes the instance of the alilogs logging driver
func (ls *logStream) Close() error {
	ls.lock.Lock()
	defer ls.lock.Unlock()
	if !ls.closed {
		close(ls.messages)
	}
	ls.closed = true
	return nil
}

// newTicker is used for time-based batching.  newTicker is a variable such
// that the implementation can be swapped out for unit tests.
var newTicker = func(freq time.Duration) *time.Ticker {
	return time.NewTicker(freq)
}

// collectLogs executes as a goroutine to perform put logs for
// submission to the logstore.  Batching is performed on time- and size-
// bases.  Time-based batching occurs at a 5 second interval (defined in the
// batchPublishFrequency const).  Size-based batching is performed on the
// maximum number of logs per batch (defined in maximumLogsPerPut) and
// the maximum number of total bytes in a batch (defined in
// maximumBytesPerPut).
func (ls *logStream) collectLogs() {
	le := logrus.WithFields(logrus.Fields{
		"endpoint": ls.endpoint,
		"project":  ls.projectName,
		"logstore": ls.logstoreName,
	})

	timer := newTicker(batchPublishFrequency)
	for {
		select {
		case <-timer.C:
			ls.publishLogs()
			le.WithFields(logrus.Fields{
				"trigger": "time",
				"count":   len(ls.logGroup.Logs),
				"size":    ls.logGroup.Size(),
			}).Debug("")
			ls.logGroup.Reset()
			ls.logGroup.Topic = proto.String(ls.topic)
		case msg, more := <-ls.messages:
			if !more {
				ls.publishLogs()
				logrus.WithFields(logrus.Fields{
					"trigger": "EOF",
					"count":   len(ls.logGroup.Logs),
					"size":    ls.logGroup.Size(),
				}).Debug("")
				return
			}
			unprocessedLine := msg.Line
			logMsg := &sls.LogContent{
				Key:   proto.String("message"),
				Value: proto.String(string(unprocessedLine)),
			}
			contents := ls.extraLogContents
			contents = append(contents, logMsg)
			logRecord := sls.Log{
				Time:     proto.Uint32(uint32(time.Now().Unix())),
				Contents: contents,
			}
			if len(unprocessedLine) > 0 {
				if (len(ls.logGroup.Logs) >= maximumLogsPerPut) || (ls.logGroup.Size()+logRecord.Size() > maximumBytesPerPut) {
					// Publish an existing batch if it's already over the maximum number of logs or if adding this
					// line would push it over the maximum number of total bytes.
					ls.publishLogs()
					logrus.WithFields(logrus.Fields{
						"trigger": "size",
						"count":   len(ls.logGroup.Logs),
						"size":    ls.logGroup.Size(),
					}).Debug("")
					ls.logGroup.Reset()
					ls.logGroup.Topic = proto.String(ls.topic)
				}
				ls.logGroup.Logs = append(ls.logGroup.Logs, &logRecord)
			}
		}
	}
}

// publishLogs calls PutLogs for a given LogGroup
func (ls *logStream) publishLogs() {
	err := ls.client.PutLogs(ls.logGroup)
	if err != nil {
		le := logrus.WithFields(logrus.Fields{
			"endpoint": ls.endpoint,
			"project":  ls.projectName,
			"logstore": ls.logstoreName,
		})
		if serviceErr, ok := err.(sls.Error); ok {
			le.WithFields(logrus.Fields{
				"errorCode":    serviceErr.Code,
				"errorMessage": serviceErr.Message,
			}).Error("PutLogs occurs sls error")
		} else {
			le.Error("PutLogs occurs err:", err)
		}
	}
}

// ValidateLogOpt looks for alilogs-specific log options
func ValidateLogOpt(cfg map[string]string) error {
	for key := range cfg {
		switch key {
		case endpointKey, projectKey, logstoreKey, labelsKey, envKey:
		default:
			return fmt.Errorf("unknown log opt '%s' for %s log driver", key, name)
		}
	}
	if cfg[endpointKey] == "" {
		return fmt.Errorf("must specify a value for log opt '%s'", endpointKey)
	}
	if cfg[projectKey] == "" {
		return fmt.Errorf("must specify a value for log opt '%s'", projectKey)
	}
	if cfg[logstoreKey] == "" {
		return fmt.Errorf("must specify a value for log opt '%s'", logstoreKey)
	}
	return nil
}

func parseContext(ctx *logger.Context) (*contextParams, error) {
	input := &contextParams{
		accessKeyID:     "",
		accessKeySecret: "",
		securityToken:   "",
		topicName:       "",
		extraContents:   []*sls.LogContent{},
	}
	extra := ctx.ExtraAttributes(nil)
	value, ok := extra[accessKeyIDEnvKey]
	if ok {
		input.accessKeyID = value
		delete(extra, accessKeyIDEnvKey)
	} else {
		return nil, fmt.Errorf("must specify a value for env '%s'", accessKeyIDEnvKey)
	}

	value, ok = extra[accessKeySecretEnvKey]
	if ok {
		input.accessKeySecret = value
		delete(extra, accessKeySecretEnvKey)
	} else {
		return nil, fmt.Errorf("must specify a value for env '%s'", accessKeySecretEnvKey)
	}

	if value, ok = extra[securityTokenEnvKey]; ok {
		input.securityToken = value
		delete(extra, securityTokenEnvKey)
	}

	if value, ok = extra[topicEnvKey]; ok {
		input.topicName = value
		delete(extra, topicEnvKey)
	}

	// add extra contents to log record
	for key, value := range extra {
		logContent := &sls.LogContent{
			Key:   proto.String(key),
			Value: proto.String(value),
		}
		input.extraContents = append(input.extraContents, logContent)
	}
	return input, nil
}
