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

package main

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestGetFormat(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{
		{
			format:   "env",
			expected: "env",
		},
		{
			format:   "go",
			expected: "go",
		},
		{
			format:   "json",
			expected: "json",
		},
		{
			format:   "mk",
			expected: "mk",
		},
		{
			format:   "rpm",
			expected: "rpm",
		},
		{
			format:   "ver",
			expected: "ver",
		},
		{
			format:   "tpl",
			expected: "tpl",
		},
		{
			format:   "unknown",
			expected: "tpl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			actual := getFormat(tt.format)
			if actual != tt.expected {
				t.Errorf("expected %s, but got %s", tt.expected, actual)
			}
		})
	}
}

func TestGetGoOS(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		want   string
	}{
		{
			name:   "XGOOS set",
			envVar: "linux",
			want:   "linux",
		},
		{
			name:   "XGOOS not set",
			envVar: "",
			want:   runtime.GOOS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("XGOOS", tt.envVar)
			defer os.Unsetenv("XGOOS")

			if got := getGoOS(); got != tt.want {
				t.Errorf("getGoOS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetGoArch(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{
			name: "Test with XGOARCH env set",
			env:  "arm",
			want: "arm",
		},
		{
			name: "Test with XGOARCH env not set",
			env:  "",
			want: runtime.GOARCH,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("XGOARCH", tt.env)
			if got := getGoArch(); got != tt.want {
				t.Errorf("getGoArch() = %v, want %v", got, tt.want)
			}
			os.Unsetenv("XGOARCH")
		})
	}
}

func TestGetBuildNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "Empty input",
			input:    []string{},
			expected: "",
		},
		{
			name:     "Input with no build number",
			input:    []string{"a", "b", "c", "d", "e"},
			expected: "",
		},
		{
			name:     "Input with build number from environment variable",
			input:    []string{"a", "b", "c", "d", "e", "123"},
			expected: "123",
		},
		{
			name:     "Input with build number from input",
			input:    []string{"a", "b", "c", "d", "e", "123"},
			expected: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("BUILD_NUMBER", tt.expected)
			defer os.Unsetenv("BUILD_NUMBER")

			result := getBuildNumber(tt.input)
			if result != tt.expected {
				t.Errorf("getBuildNumber() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetBuildType(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "BUILD_TYPE is set",
			envValue: "Debug",
			want:     "Debug",
		},
		{
			name:     "BUILD_TYPE is not set",
			envValue: "",
			want:     "X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("BUILD_TYPE", tt.envValue)
			defer os.Unsetenv("BUILD_TYPE")

			if got := getBuildType(); got != tt.want {
				t.Errorf("getBuildType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doExec(t *testing.T) {
	type args struct {
		cmd  string
		args []string
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "test doExec",
			args: args{
				cmd:  "false",
				args: []string{},
			},
			want:    []byte(""),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := doExec(tt.args.cmd, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("doExec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("doExec returned unexpected output: got %s, want %s", string(got), string(tt.want))
			}
		})
	}
}

func TestSemverString(t *testing.T) {
	v := &semver{
		Major:       1,
		Minor:       2,
		Patch:       3,
		Build:       4,
		Notes:       "notes",
		Type:        "type",
		Dirty:       true,
		Sha7:        "sha7",
		Sha32:       "sha32",
		Epoch:       5,
		SemVer:      "semver",
		SemVerRPM:   "semverRPM",
		BuildDate:   "buildDate",
		ReleaseDate: "releaseDate",
	}

	expected := "1.2.3-notes+4+dirty"
	actual := v.String()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestSemverRPM(t *testing.T) {
	v := &semver{
		Major:       1,
		Minor:       2,
		Patch:       3,
		Build:       4,
		Notes:       "notes",
		Type:        "type",
		Dirty:       true,
		Sha7:        "sha7",
		Sha32:       "sha32",
		Epoch:       5,
		SemVer:      "semver",
		SemVerRPM:   "semverRPM",
		BuildDate:   "buildDate",
		ReleaseDate: "releaseDate",
	}

	expected := "1.2.3+notes+4+dirty"
	actual := v.RPM()

	if expected != actual {
		t.Errorf("Expected %s, but got %s", expected, actual)
	}
}

func TestSemver_EnvVars(t *testing.T) {
	v := &semver{
		GOOS:        "linux",
		GOARCH:      "amd64",
		OS:          "linux",
		Arch:        "x86_64",
		Major:       1,
		Minor:       2,
		Patch:       3,
		Build:       4,
		Notes:       "notes",
		Type:        "type",
		Dirty:       true,
		Sha7:        "sha7",
		Sha32:       "sha32",
		Epoch:       1643622400,
		SemVer:      "1.2.3+4",
		SemVerRPM:   "1.2.3+4",
		BuildDate:   "2023-01-23",
		ReleaseDate: "2023-01-24",
	}

	expectedEnvVars := []string{
		"GOOS=linux",
		"GOARCH=amd64",
		"OS=linux",
		"ARCH=x86_64",
		"MAJOR=1",
		"MINOR=2",
		"PATCH=3",
		"BUILD=004",
		`NOTES="notes"`,
		"TYPE=type",
		"DIRTY=true",
		"SHA7=sha7",
		"SHA32=sha32",
		"EPOCH=1643622400",
		`SEMVER="1.2.3+4"`,
		`SEMVER_RPM="1.2.3+4"`,
		`BUILD_DATE="2023-01-23"`,
		`RELEASE_DATE="2023-01-24"`,
	}

	actualEnvVars := v.EnvVars()

	if !reflect.DeepEqual(actualEnvVars, expectedEnvVars) {
		t.Errorf("Expected %v, but got %v", expectedEnvVars, actualEnvVars)
	}
}

func TestSemver_Timestamp(t *testing.T) {
	v := &semver{
		Epoch: 1643622400,
	}

	expected := time.Unix(1643622400, 0)
	actual := v.Timestamp()

	if !expected.Equal(actual) {
		t.Errorf("Expected %v, but got %v", expected, actual)
	}
}

func TestToInt(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"0", 0},
		{"1", 1},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("input %s", c.input), func(t *testing.T) {
			result := toInt(c.input)
			if result != c.expected {
				t.Errorf("expected %d, but got %d", c.expected, result)
			}
		})
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"0", 0},
		{"1", 1},
		{"1234567890", 1234567890},
	}

	for _, test := range tests {
		result := toInt64(test.input)
		if result != test.expected {
			t.Errorf("toInt64(%s) = %d, expected %d", test.input, result, test.expected)
		}
	}
}

func TestFileExists(t *testing.T) {
	// Test case: File exists
	exists := fileExists("./semver.go")
	if !exists {
		t.Errorf("fileExists(\"./semver.go\") = %t, want %t", exists, true)
	}

	// Test case: File does not exist
	exists = fileExists("./nonexistent.txt")
	if exists {
		t.Errorf("fileExists(\"./nonexistent.txt\") = %t, want %t", exists, false)
	}
}
