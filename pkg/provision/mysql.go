package provision

import (
	"context"
	"encoding/json"

	everestv1alpha "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/percona-everest-backend/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MySQL implements logic for the MySQL command.
type MySQL struct {
	config        *MySQLConfig
	everestClient everestClientConnector
	l             *logrus.Entry
}

// MySQLConfig stores configuration for the MySQL command.
type MySQLConfig struct {
	Name         string
	KubernetesID string `mapstructure:"kubernetes-id"`

	Everest struct {
		// Endpoint stores URL to Everest.
		Endpoint string
	}

	DB struct {
		Version string
	}

	Nodes  int
	CPU    string
	Memory string
	Disk   string

	ExternalAccess bool `mapstructure:"external-access"`
}

// NewMySQL returns a new MySQL struct.
func NewMySQL(c *MySQLConfig, everestClient everestClientConnector) *MySQL {
	if c == nil {
		logrus.Panic("MySQLConfig is required")
	}

	cli := &MySQL{
		config:        c,
		everestClient: everestClient,
		l:             logrus.WithField("component", "provision/mysql"),
	}

	return cli
}

// Run runs the MySQL command.
func (m *MySQL) Run(ctx context.Context) error {
	m.l.Info("Preparing cluster config")
	body, err := m.prepareBody()
	if err != nil {
		return err
	}

	m.l.Infof("Creating %q database cluster", m.config.Name)
	_, err = m.everestClient.CreateDBCluster(ctx, m.config.KubernetesID, *body)
	if err != nil {
		return err
	}

	m.l.Infof("Database cluster %q has been scheduled to Kubernetes", m.config.Name)

	return nil
}

func (m *MySQL) prepareBody() (*client.DatabaseCluster, error) {
	cpu, err := resource.ParseQuantity(m.config.CPU)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse cpu")
	}

	memory, err := resource.ParseQuantity(m.config.Memory)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse memory")
	}

	disk, err := resource.ParseQuantity(m.config.Disk)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse disk storage")
	}

	replicas := int32(m.config.Nodes)
	version := m.config.DB.Version
	if m.config.DB.Version == "latest" {
		// An empty string means the operator uses the latest version
		version = ""
	}

	payload := everestv1alpha.DatabaseCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "everest.percona.com/v1alpha1",
			Kind:       "DatabaseCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: m.config.Name,
		},
		Spec: everestv1alpha.DatabaseClusterSpec{
			Engine: everestv1alpha.Engine{
				Type:     everestv1alpha.DatabaseEnginePXC,
				Replicas: replicas,
				Version:  version,
				Storage: everestv1alpha.Storage{
					Size: disk,
				},
				Resources: everestv1alpha.Resources{
					CPU:    cpu,
					Memory: memory,
				},
			},
			Proxy: everestv1alpha.Proxy{
				Type:     everestv1alpha.ProxyTypeHAProxy,
				Replicas: &replicas,
				Expose: everestv1alpha.Expose{
					Type: everestv1alpha.ExposeTypeInternal,
				},
			},
		},
	}

	if m.config.ExternalAccess {
		m.l.Debug("Enabling external access")
		payload.Spec.Proxy.Expose.Type = everestv1alpha.ExposeTypeExternal
	}

	return m.convertPayload(payload)
}

func (m *MySQL) convertPayload(payload everestv1alpha.DatabaseCluster) (*client.DatabaseCluster, error) {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal payload to json")
	}

	m.l.Debug(string(bodyJSON))

	body := &client.DatabaseCluster{}
	err = json.Unmarshal(bodyJSON, body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal payload back to json")
	}

	return body, nil
}