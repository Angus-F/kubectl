/*
Copyright 2018 The Kubernetes Authors.

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

package rollout

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"github.com/Angus-F/cli-runtime/pkg/genericclioptions"
	restclient "github.com/Angus-F/client-go/rest"
	"github.com/Angus-F/client-go/rest/fake"
	cmdtesting "github.com/Angus-F/kubectl/pkg/cmd/testing"
	"github.com/Angus-F/kubectl/pkg/scheme"
)

var rolloutPauseGroupVersionEncoder = schema.GroupVersion{Group: "apps", Version: "v1"}
var rolloutPauseGroupVersionDecoder = schema.GroupVersion{Group: "apps", Version: "v1"}

func TestRolloutPause(t *testing.T) {
	deploymentName := "deployment/nginx-deployment"
	ns := scheme.Codecs.WithoutConversion()
	tf := cmdtesting.NewTestFactory().WithNamespace("test")

	info, _ := runtime.SerializerInfoForMediaType(ns.SupportedMediaTypes(), runtime.ContentTypeJSON)
	encoder := ns.EncoderForVersion(info.Serializer, rolloutPauseGroupVersionEncoder)
	tf.Client = &RolloutPauseRESTClient{
		RESTClient: &fake.RESTClient{
			GroupVersion:         rolloutPauseGroupVersionEncoder,
			NegotiatedSerializer: ns,
			Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				switch p, m := req.URL.Path, req.Method; {
				case p == "/namespaces/test/deployments/nginx-deployment" && (m == "GET" || m == "PATCH"):
					responseDeployment := &appsv1.Deployment{}
					responseDeployment.Name = deploymentName
					body := ioutil.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(encoder, responseDeployment))))
					return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: body}, nil
				default:
					t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
					return nil, nil
				}
			}),
		},
	}

	streams, _, buf, _ := genericclioptions.NewTestIOStreams()
	cmd := NewCmdRolloutPause(tf, streams)

	cmd.Run(cmd, []string{deploymentName})
	expectedOutput := "deployment.apps/" + deploymentName + " paused\n"
	if buf.String() != expectedOutput {
		t.Errorf("expected output: %s, but got: %s", expectedOutput, buf.String())
	}
}

type RolloutPauseRESTClient struct {
	*fake.RESTClient
}

func (c *RolloutPauseRESTClient) Get() *restclient.Request {
	return c.RESTClient.Verb("GET")
}

func (c *RolloutPauseRESTClient) Patch(pt types.PatchType) *restclient.Request {
	return c.RESTClient.Verb("PATCH")
}
