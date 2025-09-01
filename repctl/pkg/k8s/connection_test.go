/*
 *
 * Copyright © 2021-2024 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

/*
 Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package k8s

import (
	"errors"
	"testing"

	repv1 "github.com/dell/csm-replication/api/v1"
	"github.com/stretchr/testify/assert"
	apiExtensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetControllerRuntimeClient(t *testing.T) {
	originalClientNewFunc := clientNewFunc
	originalBuildConfigFromFlags := buildConfigFromFlags

	defer func() {
		clientNewFunc = originalClientNewFunc
		buildConfigFromFlags = originalBuildConfigFromFlags
	}()

	type testCase struct {
		name       string
		kubeconfig string
		setupMocks func()
		wantErr    bool
		errMsg     string
	}

	testCases := []testCase{
		{
			name:       "Valid kubeconfig",
			kubeconfig: createTempKubeconfig(t),
			setupMocks: func() {
				clientNewFunc = func(config *rest.Config, options client.Options) (client.Client, error) {
					return &mockClient{}, nil
				}
				buildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
					return &rest.Config{}, nil
				}
			},
			wantErr: false,
		},
		{
			name:       "Invalid kubeconfig path",
			kubeconfig: "invalid/path/to/kubeconfig",
			setupMocks: func() {
				buildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
					return nil, errors.New("failed to build config from flags")
				}
			},
			wantErr: true,
			errMsg:  "failed to build config from flags",
		},
		{
			name:       "Error in client creation",
			kubeconfig: createTempKubeconfig(t),
			setupMocks: func() {
				clientNewFunc = func(config *rest.Config, options client.Options) (client.Client, error) {
					return nil, errors.New("client creation error")
				}
				buildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
					return &rest.Config{}, nil
				}
			},
			wantErr: true,
			errMsg:  "client creation error",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(apiExtensionsv1.AddToScheme(scheme))
			utilruntime.Must(repv1.AddToScheme(scheme))

			k8sClient, err := getControllerRuntimeClient(tt.kubeconfig)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, k8sClient)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, k8sClient)
			}
		})
	}
}

type mockClient struct {
	client.Client
}

// Helper function to create a temporary kubeconfig file
func createTempKubeconfig(t *testing.T) string {
	return "/path/to/temp/kubeconfig"
}
