package common

import (
	"github.com/stretchr/testify/suite"
	"testing"
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

func TestConstantsTestSuite(t *testing.T) {
	testSuite := new(ConstantsTestSuite)
	suite.Run(t, testSuite)
}
