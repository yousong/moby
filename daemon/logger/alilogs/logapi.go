// Pakcage alilogs api interface

package alilogs

import (
	"errors"

	"github.com/Sirupsen/logrus"
	"github.com/galaxydi/go-loghub"
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
	logStore, err := logProject.GetLogStore(logstoreName)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Could not get ali logstore")
		return nil, errors.New("Could not get ali logstore")
	}
	return logStore, nil
}

func (client *AliLogClient) getLogProject(projectName, endpoint, accessKeyID, accessKeySecret, securityToken string) (*sls.LogProject, error) {
	logProject, err := sls.NewLogProject(projectName, endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Could not get ali log project")
		return nil, errors.New("Could not get ali log project")
	}
	if securityToken != "" {
		logProject.WithToken(securityToken)
	}
	return logProject, nil
}
