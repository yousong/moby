package alilogs

import (
	"sync"
	"testing"
	"time"

	sls "github.com/aliyun-fc/go-loghub"
	"github.com/docker/docker/daemon/logger"
	"github.com/gogo/protobuf/proto"
)

func TestCollectLogsNumberLimit(t *testing.T) {
	extraContents := []*sls.LogContent{}
	mockClient := NewMockClient()
	mockClient.ErrType = NoError

	stream := &logStream{
		endpoint:         "test-endpoint",
		projectName:      "test-project",
		logstoreName:     "test-logstore",
		topic:            "demo_topic",
		extraLogContents: extraContents,
		client:           mockClient,
		logGroup: &sls.LogGroup{
			Topic: proto.String("demo_topic"),
			Logs:  []*sls.Log{},
		},
		messages: make(chan *logger.Message, maximumLogsPerPut),
	}

	go stream.collectLogs()

	var wg sync.WaitGroup
	wg.Add(maximumLogsPerPut + 102)

	for i := 0; i < maximumLogsPerPut+102; i++ {
		go worker(stream, &wg)
	}
	wg.Wait()
	time.Sleep(batchPublishFrequency)
	stream.Close()
	if mockClient.Topic != "demo_topic" {
		t.Errorf("check topic fail, expect:%s, actual:%s", stream.topic, mockClient.Topic)
	}
	if len(mockClient.Logs) != maximumLogsPerPut+102 {
		t.Errorf("check log number fail, expect:%v, actual:%v", maximumLogsPerPut+102, len(mockClient.Logs))
	}

}

func worker(stream *logStream, wg *sync.WaitGroup) {
	stream.Log(&logger.Message{
		Line:      []byte("test log"),
		Timestamp: time.Time{},
	})
	wg.Done()
}

func TestValidateOpt(t *testing.T) {
	// endpointKey, projectKey, logstoreKey, labelsKey, envKey
	opt := map[string]string{}
	opt[endpointKey] = ""
	err := ValidateLogOpt(opt)
	if err == nil {
		t.Errorf("check log opt fail: %v", opt)
	}

	opt[endpointKey] = "test-endpoint"
	opt[projectKey] = ""
	err = ValidateLogOpt(opt)
	if err == nil {
		t.Errorf("check log opt fail: %v", opt)
	}

	opt[projectKey] = "test-project"
	opt[logstoreKey] = ""
	err = ValidateLogOpt(opt)
	if err == nil {
		t.Errorf("check log opt fail: %v", opt)
	}

	opt[logstoreKey] = "test-logstore"
	opt[labelsKey] = "attr1,attr2"
	opt[envKey] = "e1=v1,e2=v2"
	err = ValidateLogOpt(opt)
	if err != nil {
		t.Errorf("check log opt fail: %v", opt)
	}

	opt["error-key"] = "unsupported"
	err = ValidateLogOpt(opt)
	if err == nil {
		t.Errorf("check log opt fail: %v", opt)
	}
}

func TestCollectLogsSimple(t *testing.T) {
	ec1 := &sls.LogContent{
		Key:   proto.String("ex1"),
		Value: proto.String("ex1 value"),
	}
	ec2 := &sls.LogContent{
		Key:   proto.String("ex2"),
		Value: proto.String("ex2 value"),
	}
	extraContents := []*sls.LogContent{ec1, ec2}
	mockClient := NewMockClient()
	stream := &logStream{
		endpoint:         "test-endpoint",
		projectName:      "test-project",
		logstoreName:     "test-logstore",
		topic:            "demo_topic",
		extraLogContents: extraContents,
		client:           mockClient,
		logGroup: &sls.LogGroup{
			Topic: proto.String("demo_topic"),
			Logs:  []*sls.Log{},
		},
		messages: make(chan *logger.Message, maximumLogsPerPut),
	}

	ticks := make(chan time.Time)
	newTicker = func(_ time.Duration) *time.Ticker {
		return &time.Ticker{
			C: ticks,
		}
	}

	go stream.collectLogs()

	stream.Log(&logger.Message{
		Line:      []byte("this is test log 1"),
		Timestamp: time.Time{},
	})
	stream.Log(&logger.Message{
		Line:      []byte("this is test log 2"),
		Timestamp: time.Time{},
	})
	stream.Log(&logger.Message{
		Line:      []byte("this is test log 3"),
		Timestamp: time.Time{},
	})

	ticks <- time.Time{}
	stream.Close()

	// Wait a moment for the logs were writted into mockClient
	time.Sleep(1 * time.Second)

	if len(mockClient.Logs) != 3 {
		t.Errorf("should be 3 number logs, actual log numbers: %v", len(mockClient.Logs))
	}
}

func TestPublishLogs(t *testing.T) {
	ec1 := &sls.LogContent{
		Key:   proto.String("ex1"),
		Value: proto.String("ex1 value"),
	}
	ec2 := &sls.LogContent{
		Key:   proto.String("ex2"),
		Value: proto.String("ex2 value"),
	}
	extraContents := []*sls.LogContent{ec1, ec2}
	mockClient := NewMockClient()
	stream := &logStream{
		endpoint:         "test-endpoint",
		projectName:      "test-project",
		logstoreName:     "test-logstore",
		topic:            "demo_topic",
		extraLogContents: extraContents,
		client:           mockClient,
		logGroup: &sls.LogGroup{
			Topic: proto.String("demo_topic"),
			Logs:  []*sls.Log{},
		},
		messages: make(chan *logger.Message, maximumLogsPerPut),
	}

	logMsg := &sls.LogContent{
		Key:   proto.String("message"),
		Value: proto.String(string("this is a log")),
	}
	contents := stream.extraLogContents
	contents = append(contents, logMsg)
	logRecord := sls.Log{
		Time:     proto.Uint32(uint32(time.Now().Unix())),
		Contents: contents,
	}
	stream.logGroup.Logs = append(stream.logGroup.Logs, &logRecord)
	mockClient.ErrType = NoError
	stream.publishLogs()

	mockClient.ErrType = InternalServerError
	stream.publishLogs()

	mockClient.ErrType = UnknownError
	stream.publishLogs()
}

func TestNewContainerStream(t *testing.T) {
	extraContents := []*sls.LogContent{}
	containerStream := &logStream{
		topic:            "demo_topic",
		extraLogContents: extraContents,
		client:           NewMockClient(),
		messages:         make(chan *logger.Message, maximumLogsPerPut),
	}
	if containerStream == nil {
		t.Errorf("failed to new containerStream\n")
	}
	if containerStream.Name() != "alilogs" {
		t.Errorf("error logger name: %s", containerStream.Name())
	}

	containerStream.Log(&logger.Message{
		Line:      []byte("this is one log"),
		Timestamp: time.Time{},
	})
	msg := containerStream.messages
	if msg == nil {
		t.Errorf("stream should has one log")
	}
	err := containerStream.Close()
	if err != nil {
		t.Errorf("stream should be close successful, err: %v", err)
	}
	if containerStream.closed != true {
		t.Errorf("stream should be closed, close flag: %v", containerStream.closed)
	}
}

func TestParseContext(t *testing.T) {
	envSlice := []string{"accessKeyID=mock_id", "accessKeySecret=mock_key", "securityToken=mock_token", "topic=mock_topic"}
	labelMap := map[string]string{}
	labelMap["a1"] = "v1"
	labelMap["a2"] = "v2"
	labelMap["a3"] = "v3"

	ctx := logger.Context{
		Config: map[string]string{
			endpointKey: "log.cn-hangzhou.aliyuncs.com",
			projectKey:  "test-project",
			logstoreKey: "test-logstore",
			"env":       "accessKeyID,accessKeySecret,securityToken,topic",
			"labels":    "a1,a2,a3",
		},
		ContainerEnv:    envSlice,
		ContainerLabels: labelMap,
	}
	input, err := parseContext(&ctx)
	if err != nil {
		t.Errorf("failed to parse context")
	}
	if input.accessKeyID != "mock_id" {
		t.Errorf("parse accessKeyID fail:%s", input.accessKeyID)
	}
	if input.accessKeySecret != "mock_key" {
		t.Errorf("parse accessKeySecret fail:%s", input.accessKeySecret)
	}
	if input.topicName != "mock_topic" {
		t.Errorf("parse topic fail:%s", input.topicName)
	}
	if len(input.extraContents) != 3 {
		t.Errorf("parse extraContents fail:%v", input.extraContents)
	}
}

func TestParseContextError(t *testing.T) {
	envSlice := []string{"accessKeySecret=mock_key", "securityToken=mock_token", "topic=mock_topic"}
	labelMap := map[string]string{}
	labelMap["a1"] = "v1"

	ctx := logger.Context{
		Config: map[string]string{
			endpointKey: "log.cn-hangzhou.aliyuncs.com",
			projectKey:  "test-project",
			logstoreKey: "test-logstore",
			"env":       "accessKeyID,accessKeySecret,securityToken,topic",
			"labels":    "a1,a2,a3",
		},
		ContainerEnv:    envSlice,
		ContainerLabels: labelMap,
	}
	_, err := parseContext(&ctx)
	if err == nil {
		t.Errorf("invalid accessKeyID")
	}

	envSlice = []string{"accessKeyID=mock_id", "securityToken=mock_token", "topic=mock_topic"}
	ctx = logger.Context{
		Config: map[string]string{
			endpointKey: "log.cn-hangzhou.aliyuncs.com",
			projectKey:  "test-project",
			logstoreKey: "test-logstore",
			"env":       "accessKeyID,accessKeySecret,securityToken,topic",
			"labels":    "a1,a2,a3",
		},
		ContainerEnv:    envSlice,
		ContainerLabels: labelMap,
	}
	_, err = parseContext(&ctx)
	if err == nil {
		t.Errorf("invalid accessKeySecret")
	}

	envSlice = []string{"accessKeyID=mock_id", "accessKeySecret=mock_key"}
	ctx = logger.Context{
		Config: map[string]string{
			endpointKey: "log.cn-hangzhou.aliyuncs.com",
			projectKey:  "test-project",
			logstoreKey: "test-logstore",
			"env":       "accessKeyID,accessKeySecret,securityToken,topic",
			"labels":    "a1,a2,a3",
		},
		ContainerEnv:    envSlice,
		ContainerLabels: labelMap,
	}
	input, _ := parseContext(&ctx)
	if input.securityToken != "" {
		t.Errorf("token should be empty")
	}
	if input.topicName != "" {
		t.Errorf("topic should be empty")
	}
}
