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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi"
)

// Options server options
type Options struct {
	Port     string
	BindAddr string
	StubPath string
}

const (
	// DefaultAddress address of stub server
	DefaultAddress = "localhost"
	// DefaultPort port of stub server
	DefaultPort = "4771"
)

// RunStubServer runs mock grpc server
func RunStubServer(opt Options) {
	if opt.Port == "" {
		opt.Port = DefaultPort
	}
	if opt.BindAddr == "" {
		opt.BindAddr = DefaultAddress
	}
	addr := opt.BindAddr + ":" + opt.Port
	r := chi.NewRouter()
	r.Post("/add", addStub)
	r.Get("/", listStub)
	r.Post("/find", handleFindStub)
	r.Get("/clear", handleClearStub)

	if opt.StubPath != "" {
		readStubFromFile(opt.StubPath)
	}

	fmt.Println("Serving stub admin on http://" + addr)
	go func() {
		server := &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 3 * time.Second,
			Handler:           r,
		}
		err := server.ListenAndServe()
		log.Println(err)
	}()
}

func responseError(err error, w http.ResponseWriter) {
	w.WriteHeader(500)
	_, e := w.Write([]byte(err.Error()))
	if e != nil {
		log.Printf("error writing error %s to response: %s\n", err.Error(), e.Error())
	}
}

// Stub structure
type Stub struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Input   Input  `json:"input"`
	Output  Output `json:"output"`
}

// Input structure
type Input struct {
	Equals   map[string]interface{} `json:"equals"`
	Contains map[string]interface{} `json:"contains"`
	Matches  map[string]interface{} `json:"matches"`
}

// Output structure
type Output struct {
	Data  map[string]interface{} `json:"data"`
	Error string                 `json:"error"`
}

func addStub(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		responseError(err, w)
		return
	}

	stub := new(Stub)
	err = json.Unmarshal(body, stub)
	if err != nil {
		responseError(err, w)
		return
	}

	err = validateStub(stub)
	if err != nil {
		responseError(err, w)
		return
	}

	err = storeStub(stub)
	if err != nil {
		responseError(err, w)
		return
	}

	_, err = w.Write([]byte("Success add stub"))
	if err != nil {
		log.Printf("error writing success to response: %s\n", err.Error())
	}
}

func listStub(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(allStub())
	if err != nil {
		log.Printf("error encoding stub: %s\n", err.Error())
	}
}

func validateStub(stub *Stub) error {
	if stub.Service == "" {
		return fmt.Errorf("Service name can't be empty")
	}

	if stub.Method == "" {
		return fmt.Errorf("Method name can't be emtpy")
	}

	// due to golang implementation
	// method name must capital
	stub.Method = strings.Title(stub.Method)

	switch {
	case stub.Input.Contains != nil:
		break
	case stub.Input.Equals != nil:
		break
	case stub.Input.Matches != nil:
		break
	default:
		return fmt.Errorf("Input cannot be empty")
	}

	// TODO: validate all input case

	if stub.Output.Error == "" && stub.Output.Data == nil {
		return fmt.Errorf("Output can't be empty")
	}
	return nil
}

type findStubPayload struct {
	Service string                 `json:"service"`
	Method  string                 `json:"method"`
	Data    map[string]interface{} `json:"data"`
}

func handleFindStub(w http.ResponseWriter, r *http.Request) {
	stub := new(findStubPayload)
	err := json.NewDecoder(r.Body).Decode(stub)
	if err != nil {
		responseError(err, w)
		return
	}

	// due to golang implementation
	// method name must capital
	stub.Method = strings.Title(stub.Method)

	output, err := findStub(stub)
	if err != nil {
		log.Println(err)
		responseError(err, w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(output)
	if err != nil {
		log.Printf("error encoding output: %s\n", err.Error())
	}
}

func handleClearStub(w http.ResponseWriter, _ *http.Request) {
	clearStorage()
	_, err := w.Write([]byte("OK"))
	if err != nil {
		log.Printf("error writing `OK` to response: %s\n", err.Error())
	}
}
