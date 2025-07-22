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

// revive:disable:var-naming
package logger

// revive:enable:var-naming

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/suite"
)

type ConstantsTestSuite struct {
	suite.Suite
}

func (suite *ConstantsTestSuite) TestParseLevel() {
	logLevels := []string{"panic", "fatal", "error", "warn", "info", "debug", "trace"}

	for _, val := range logLevels {
		_, err := ParseLevel(val)
		suite.NoError(err)
	}
}

func (suite *ConstantsTestSuite) TestParseLevelInvalid() {
	_, err := ParseLevel("invalid")
	suite.Error(err, "not a valid logrus level")
}

func (suite *ConstantsTestSuite) TestGetLoggerFromContext() {
	// Test case: Context with logger
	ctxWithLogger := context.WithValue(context.Background(), LoggerContextKey, logr.Discard())
	logger := GetLoggerFromContext(ctxWithLogger)
	suite.NotNil(logger)

	// Test case: Context without logger
	ctxWithoutLogger := context.Background()
	logger = GetLoggerFromContext(ctxWithoutLogger)
	suite.NotNil(logger)
}

func TestConstantsTestSuite(t *testing.T) {
	testSuite := new(ConstantsTestSuite)
	suite.Run(t, testSuite)
}
