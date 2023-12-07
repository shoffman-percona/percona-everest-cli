// percona-everest-cli
// Copyright (C) 2023 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package password holds the main logic for password commands.
package password

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"

	"github.com/dchest/uniuri"
	"go.uber.org/zap"
	"golang.org/x/crypto/pbkdf2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/percona/percona-everest-cli/pkg/kubernetes"
)

// Reset implements the main logic for command.
type Reset struct {
	config ResetConfig
	l      *zap.SugaredLogger

	kubeClient *kubernetes.Kubernetes
}

type (
	// ResetConfig stores configuration for the reset command.
	ResetConfig struct {
		// KubeconfigPath is a path to a kubeconfig
		KubeconfigPath string `mapstructure:"kubeconfig"`
		// Namespace defines the namespace password shall be reset in.
		Namespace string
	}

	// ResetResponse is a response from the reset command.
	ResetResponse struct {
		// Passwod is plain-text password generated by the command.
		Password string `json:"password,omitempty"`
	}
)

const passwordSecretName = "everest-password"

func (r ResetResponse) String() string {
	return fmt.Sprintf("Your new password is:\n%s", r.Password)
}

// NewReset returns a new Reset struct.
func NewReset(c ResetConfig, l *zap.SugaredLogger) (*Reset, error) {
	cli := &Reset{
		config: c,
		l:      l.With("component", "password/reset"),
	}

	k, err := kubernetes.New(c.KubeconfigPath, cli.l)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			cli.l.Error("Could not connect to Kubernetes. " +
				"Make sure Kubernetes is running and is accessible from this computer/server.")
		}
		return nil, err
	}
	cli.kubeClient = k

	return cli, nil
}

// Run runs the reset command.
func (r *Reset) Run(ctx context.Context) (*ResetResponse, error) {
	ns, err := r.kubeClient.GetNamespace(ctx, r.config.Namespace)
	if err != nil {
		return nil, errors.Join(err, errors.New("could not get namespace from Kubernetes"))
	}

	newPassword := uniuri.NewLen(128)
	salt := []byte(ns.UID)
	hash := pbkdf2.Key([]byte(newPassword), salt, 4096, 32, sha256.New)

	err = r.kubeClient.SetSecret(&corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      passwordSecretName,
			Namespace: r.config.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": hash,
		},
	})
	if err != nil {
		return nil, errors.Join(err, errors.New("could not update password in Kubernetes"))
	}

	return &ResetResponse{Password: newPassword}, nil
}