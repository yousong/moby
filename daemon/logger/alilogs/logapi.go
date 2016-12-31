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
	Endpoint     string
	ProjectName  string
	LogstoreName string
	logStore     *sls.LogStore
}

// PutLogs implements ali PutLogs method
func (client *AliLogClient) PutLogs(logGroup *sls.LogGroup) error {
	return client.logStore.PutLogs(logGroup)
}

// NewAliLogClient ...
func NewAliLogClient(endpoint, projectName, logstoreName, accessKeyID, accessKeySecret, securityToken string) (AliLogAPI, error) {
	client := AliLogClient{}
	client.Endpoint = endpoint
	client.ProjectName = projectName
	client.LogstoreName = logstoreName

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

	client.logStore, err = logProject.GetLogStore(logstoreName)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Could not get ali logstore")
		return nil, errors.New("Could not get ali logstore")
	}

	logrus.WithFields(logrus.Fields{
		"endpoint":     endpoint,
		"projectName":  projectName,
		"logstoreName": logstoreName,
	}).Info("Created alilogs client")

	return &client, nil
}
