/*
Copyright © 2021-2023 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package stub

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStub(t *testing.T) {
	type test struct {
		name    string
		mock    func() *http.Request
		handler http.HandlerFunc
		expect  string
	}

	cases := []test{
		{
			name: "add stub simple",
			mock: func() *http.Request {
				payload := `{
						"service": "Testing",
						"method":"TestMethod",
						"input":{
							"equals":{
								"Hola":"Mundo"
							}
						},
						"output":{
							"data":{
								"Hello":"World"
							}
						}
					}`
				read := bytes.NewReader([]byte(payload))
				return httptest.NewRequest("POST", "/add", read)
			},
			handler: addStub,
			expect:  `Success add stub`,
		},
		{
			name: "list stub",
			mock: func() *http.Request {
				return httptest.NewRequest("GET", "/", nil)
			},
			handler: listStub,
			expect:  "{\"Testing\":{\"TestMethod\":[{\"Input\":{\"equals\":{\"Hola\":\"Mundo\"},\"contains\":null,\"matches\":null},\"Output\":{\"data\":{\"Hello\":\"World\"},\"error\":\"\"}}]}}\n",
		},
		{
			name: "find stub equals",
			mock: func() *http.Request {
				payload := `{"service":"Testing","method":"TestMethod","data":{"Hola":"Mundo"}}`
				return httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(payload)))
			},
			handler: handleFindStub,
			expect:  "{\"data\":{\"Hello\":\"World\"},\"error\":\"\"}\n",
		},
		{
			name: "add stub contains",
			mock: func() *http.Request {
				payload := `{
								"service": "Testing",
								"method":"TestMethod",
								"input":{
									"contains":{
										"field1":"hello field1",
										"field3":"hello field3"
									}
								},
								"output":{
									"data":{
										"hello":"world"
									}
								}
							}`
				return httptest.NewRequest("POST", "/add", bytes.NewReader([]byte(payload)))
			},
			handler: addStub,
			expect:  `Success add stub`,
		},
		{
			name: "find stub contains",
			mock: func() *http.Request {
				payload := `{
						"service":"Testing",
						"method":"TestMethod",
						"data":{
							"field1":"hello field1",
							"field2":"hello field2",
							"field3":"hello field3"
						}
					}`
				return httptest.NewRequest("GET", "/find", bytes.NewReader([]byte(payload)))
			},
			handler: handleFindStub,
			expect:  "{\"data\":{\"hello\":\"world\"},\"error\":\"\"}\n",
		},
		{
			name: "add stub matches regex",
			mock: func() *http.Request {
				payload := `{
						"service":"Testing2",
						"method":"TestMethod",
						"input":{
							"matches":{
								"field1":".*ello$"
							}
						},
						"output":{
							"data":{
								"reply":"OK"
							}
						}
					}`
				return httptest.NewRequest("POST", "/add", bytes.NewReader([]byte(payload)))
			},
			handler: addStub,
			expect:  "Success add stub",
		},
		{
			name: "find stub matches regex",
			mock: func() *http.Request {
				payload := `{
						"service":"Testing2",
						"method":"TestMethod",
						"data":{
							"field1":"hello"
						}
					}`
				return httptest.NewRequest("GET", "/find", bytes.NewReader([]byte(payload)))
			},
			handler: handleFindStub,
			expect:  "{\"data\":{\"reply\":\"OK\"},\"error\":\"\"}\n",
		},
		{
			name: "error find stub contains",
			mock: func() *http.Request {
				payload := `{
						"service":"Testing",
						"method":"TestMethod",
						"data":{
							"field3":"hello field4"
						}
					}`
				return httptest.NewRequest("GET", "/find", bytes.NewReader([]byte(payload)))
			},
			handler: handleFindStub,
			expect:  "Can't find stub \n\nService: Testing \n\nMethod: TestMethod \n\nInput\n\n{\n\tfield3: hello field4\n}\n\nClosest Match \n\ncontains:{\n\tfield1: hello field1\n\tfield3: hello field3\n}",
		},
		{
			name: "error find stub equals",
			mock: func() *http.Request {
				payload := `{"service":"Testing","method":"TestMethod","data":{"Hello":"World"}}`
				return httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(payload)))
			},
			handler: handleFindStub,
			expect:  "Can't find stub \n\nService: Testing \n\nMethod: TestMethod \n\nInput\n\n{\n\tHello: World\n}\n\nClosest Match \n\nequals:{\n\tHola: Mundo\n}",
		},
		{
			name: "add stub failure bad request body",
			mock: func() *http.Request {
				payload := `{
						"service": "Testing",
						"method":"TestMethod",
						"input":{
							"equals":{
								"Hola":"Mundo"
						},
						"output":{
							"data":{
								"Hello":"World"
							}
						}
					}` // malformed json; missing closing brace
				read := bytes.NewReader([]byte(payload))
				return httptest.NewRequest("POST", "/add", read)
			},
			handler: addStub,
			expect:  "unexpected end of JSON input",
		},
		{
			name: "add stub failure empty service name",
			mock: func() *http.Request {
				payload := `{
						"service": "",
						"method":"TestMethod",
						"input":{
							"equals":{
								"Hola":"Mundo"
							}
						},
						"output":{
							"data":{
								"Hello":"World"
							}
						}
					}`
				read := bytes.NewReader([]byte(payload))
				return httptest.NewRequest("POST", "/add", read)
			},
			handler: addStub,
			expect:  "Service name can't be empty",
		},
		{
			name: "add stub failure empty method name",
			mock: func() *http.Request {
				payload := `{
						"service": "Testing",
						"method":"",
						"input":{
							"equals":{
								"Hola":"Mundo"
							}
						},
						"output":{
							"data":{
								"Hello":"World"
							}
						}
					}`
				read := bytes.NewReader([]byte(payload))
				return httptest.NewRequest("POST", "/add", read)
			},
			handler: addStub,
			expect:  "Method name can't be empty",
		},
		{
			name: "add stub failure empty input",
			mock: func() *http.Request {
				payload := `{
						"service": "Testing",
						"method":"TestMethod",
						"input":{
						},
						"output":{
							"data":{
								"Hello":"World"
							}
						}
					}`
				read := bytes.NewReader([]byte(payload))
				return httptest.NewRequest("POST", "/add", read)
			},
			handler: addStub,
			expect:  "Input cannot be empty",
		},
		{
			name: "find stub failure unknown service",
			mock: func() *http.Request {
				payload := `{
						"service": "UnknownServiceName",
						"method":"TestMethod",
						"input":{
							"equals":{
								"Hola":"Mundo"
							}
						},
						"output":{
							"data":{
								"Hello":"World"
							}
						}
					}`
				read := bytes.NewReader([]byte(payload))
				return httptest.NewRequest("POST", "/add", read)
			},
			handler: handleFindStub,
			expect:  "Can't find stub for Service",
		},
	}

	for _, v := range cases {
		t.Run(v.name, func(t *testing.T) {
			wrt := httptest.NewRecorder()
			req := v.mock()
			v.handler(wrt, req)
			resp, err := io.ReadAll(wrt.Result().Body)

			assert.NoError(t, err)
			assert.Contains(t, string(resp), v.expect)
		})
	}
}

// Start and query the mock server
func TestRunStubServer(t *testing.T) {
	opt := Options{
		Port:     "4771",
		BindAddr: "0.0.0.0",
		StubPath: "../stubs",
	}

	// run the server
	RunStubServer(opt)

	// query the server. May take some time for the server
	// to start.
	queryEndpoint := "http://" + opt.BindAddr + ":" + opt.Port + "/"

	var resp *http.Response
	var err error
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		resp, err = http.Get(queryEndpoint)
		if err == nil {
			break
		}
	}

	assert.NoError(t, err, "Should successfully fetch the list of stubs")
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}
