// Pakcage alilogs api interface

package alilogs

import (
	"github.com/Sirupsen/logrus"
	"github.com/aliyun-fc/go-loghub"
)

// AliLogAPI define log api interface
type AliLogAPI interface {
	PutLogs(*sls.LogGroup) error
}

// AliLogClient implements AliLogAPI interface
type AliLogClient struct {
	logStore *sls.LogStore
}

// NewAliLogClient ...
func NewAliLogClient(endpoint, projectName, logstoreName, accessKeyID, accessKeySecret, securityToken string) (AliLogAPI, error) {
	client := AliLogClient{}
	logStore, err := client.getLogStore(endpoint, projectName, logstoreName, accessKeyID, accessKeySecret, securityToken)
	if err != nil {
		return nil, err
	}
	client.logStore = logStore

	logrus.WithFields(logrus.Fields{
		"endpoint":     endpoint,
		"projectName":  projectName,
		"logstoreName": logstoreName,
	}).Info("Created alilogs client")

	return &client, nil
}

// PutLogs implements ali PutLogs method
func (client *AliLogClient) PutLogs(logGroup *sls.LogGroup) error {
	return client.logStore.PutLogs(logGroup)
}

func (client *AliLogClient) getLogStore(endpoint, projectName, logstoreName, accessKeyID, accessKeySecret, securityToken string) (*sls.LogStore, error) {
	logProject, err := client.getLogProject(projectName, endpoint, accessKeyID, accessKeySecret, securityToken)
	if err != nil {
		return nil, err
	}
	// sls.NewLogStore returns no error
	logStore, _ := sls.NewLogStore(logstoreName, logProject)
	return logStore, nil
}

func (client *AliLogClient) getLogProject(projectName, endpoint, accessKeyID, accessKeySecret, securityToken string) (*sls.LogProject, error) {
	// sls.NewLogProject returns no error
	logProject, _ := sls.NewLogProject(projectName, endpoint, accessKeyID, accessKeySecret)
	if securityToken != "" {
		logProject.WithToken(securityToken)
	}
	return logProject, nil
}
