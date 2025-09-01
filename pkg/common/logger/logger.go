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

package logger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bombsimon/logrusr/v4"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
)

const (
	// PanicLevel represents logrus panic log level
	PanicLevel = int(logrus.PanicLevel) - 4
	// FatalLevel represents logrus fatal log level
	FatalLevel = int(logrus.FatalLevel) - 4
	// ErrorLevel represents logrus error log level
	ErrorLevel = int(logrus.ErrorLevel) - 4
	// WarnLevel represents logrus warning log level
	WarnLevel = int(logrus.WarnLevel) - 4
	// InfoLevel represents logrus info log level
	InfoLevel = int(logrus.InfoLevel) - 4
	// DebugLevel represents logrus debug log level
	DebugLevel = int(logrus.DebugLevel) - 4
	// TraceLevel represents logrus trace log level
	TraceLevel = int(logrus.TraceLevel) - 4
)

// ParseLevel returns correct logrus Level from given string name
func ParseLevel(level string) (logrus.Level, error) {
	switch strings.ToLower(level) {
	case "panic":
		return logrus.Level(PanicLevel + 4), nil
	case "fatal":
		return logrus.Level(FatalLevel + 4), nil
	case "error":
		return logrus.Level(ErrorLevel + 4), nil
	case "warn", "warning":
		return logrus.Level(WarnLevel + 4), nil
	case "info":
		return logrus.Level(InfoLevel + 4), nil
	case "debug":
		return logrus.Level(DebugLevel + 4), nil
	case "trace":
		return logrus.Level(TraceLevel + 4), nil

	}

	return logrus.Level(InfoLevel) + 4, fmt.Errorf("not a valid logrus level, falling back to InfoLevel %s", level)
}

type key int

const (
	// LoggerContextKey defines key which we use to store log in the context
	LoggerContextKey key = iota
)

// FromContext serves to pass the logger instance to the context
func FromContext(ctx context.Context) logr.Logger {
	log, ok := ctx.Value(LoggerContextKey).(logr.Logger)
	if !ok {
		logrusLog := logrus.New()
		logrusLog.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})

		logger := logrusr.New(logrusLog)
		return logger
	}
	return log
}
