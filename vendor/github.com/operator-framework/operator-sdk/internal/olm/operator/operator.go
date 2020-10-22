// Copyright 2019 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package olm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/operator-framework/operator-sdk/internal/olm"

	"github.com/spf13/pflag"
)

// TODO(estroz): figure out a good way to deal with creating scorecard objects
// and injecting proxy container

const (
	defaultTimeout = time.Minute * 2
)

// OLMCmd configures deployment and teardown of an operator via an OLM
// installation existing on a cluster.
type OLMCmd struct { // nolint:golint
	// ManifestsDir is a directory containing 1..N bundle directories and either
	// a package manifest or metadata/annotations.yaml. OperatorVersion can be
	// set to the version of the desired operator bundle and Run()/Cleanup() will
	// deploy that operator version.
	ManifestsDir string
	// OperatorVersion is the version of the operator to deploy. It must be
	// a semantic version, ex. 0.0.1.
	OperatorVersion string
	// IncludePaths are path to manifests of Kubernetes resources that either
	// supplement or override defaults generated by methods of OLMCmd. These
	// manifests can be but are not limited to: RBAC, Subscriptions,
	// CatalogSources, OperatorGroups.
	//
	// Kinds that are overridden if supplied:
	// - CatalogSource
	// - Subscription
	// - OperatorGroup
	IncludePaths []string
	// InstallMode specifies which supported installMode should be used to
	// create an OperatorGroup. The format for this field is as follows:
	//
	// "InstallModeType=[ns1,ns2[, ...]]"
	//
	// The InstallModeType string passed must be marked as "supported" in the
	// CSV being installed. The namespaces passed must exist or be created by
	// passing a Namespace manifest to IncludePaths. An empty set of namespaces
	// can be used for AllNamespaces.
	// The default mode is OwnNamespace, which uses OperatorNamespace or the
	// kubeconfig default.
	InstallMode string

	// KubeconfigPath is the local path to a kubeconfig. This uses well-defined
	// default loading rules to load the config if empty.
	KubeconfigPath string
	// OperatorNamespace is the cluster namespace in which operator resources
	// are created.
	// OperatorNamespace must already exist in the cluster or be defined in
	// a manifest passed to IncludePaths.
	OperatorNamespace string
	// OLMNamespace is the namespace in which OLM is installed.
	OLMNamespace string
	// Timeout dictates how long to wait for a REST call to complete. A call
	// exceeding Timeout will generate an error.
	Timeout time.Duration
	// ForceRegistry forces deletion of registry resources.
	ForceRegistry bool

	once sync.Once
}

var installModeFormat = "InstallModeType[=ns1,ns2[, ...]]"

func (c *OLMCmd) AddToFlagSet(fs *pflag.FlagSet) {
	prefix := "[olm only] "
	fs.StringVar(&c.OLMNamespace, "olm-namespace", olm.DefaultOLMNamespace,
		prefix+"The namespace where OLM is installed")
	fs.StringVar(&c.OperatorNamespace, "operator-namespace", "",
		prefix+"The namespace where operator resources are created. It must already exist "+
			"in the cluster or be defined in a manifest passed to --include")
	fs.StringVar(&c.ManifestsDir, "manifests", "",
		prefix+"Directory containing operator bundle directories and metadata")
	fs.StringVar(&c.OperatorVersion, "operator-version", "",
		prefix+"Version of operator to deploy")
	fs.StringVar(&c.InstallMode, "install-mode", "",
		prefix+"InstallMode to create OperatorGroup with. Format: "+installModeFormat)
	fs.StringSliceVar(&c.IncludePaths, "include", nil,
		prefix+"Path to Kubernetes resource manifests, ex. Role, Subscription. "+
			"These supplement or override defaults generated by run/cleanup")
	fs.DurationVar(&c.Timeout, "timeout", defaultTimeout,
		prefix+"Time to wait for the command to complete before failing")
}

func (c *OLMCmd) validate() error {
	if c.ManifestsDir == "" {
		return errors.New("manifests dir must be set")
	}
	if c.OperatorVersion == "" {
		return errors.New("operator version must be set")
	}
	if c.InstallMode != "" {
		if _, _, err := parseInstallModeKV(c.InstallMode); err != nil {
			return err
		}
	}
	return nil
}

func (c *OLMCmd) initialize() {
	c.once.Do(func() {
		if c.Timeout <= 0 {
			c.Timeout = defaultTimeout
		}
	})
}

func (c *OLMCmd) Run() error {
	c.initialize()
	if err := c.validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	m, err := c.newManager()
	if err != nil {
		return fmt.Errorf("error initializing operator manager: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	return m.run(ctx)
}

func (c *OLMCmd) Cleanup() (err error) {
	c.initialize()
	if err := c.validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	m, err := c.newManager()
	if err != nil {
		return fmt.Errorf("error initializing operator manager: %w", err)
	}
	// Cleanups should clean up all resources, which includes the registry.
	m.forceRegistry = true
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	return m.cleanup(ctx)
}
