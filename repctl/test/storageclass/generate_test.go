package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	// The name of the repctl binary.
	repctlCLI = "repctl"
	// The repctl "create" command used to create k8s resources.
	cmdCreate = "create"
	// Name of the resource provided to the repctl "create" command for creating
	// a storage class.
	resourceStorageClass = "sc"
	// Option used when creating StorageClasses with repctl that allows
	// user to supply a custom repctl config file for generating replication-enabled
	// storage classes.
	optFromConfig = "--from-config"
	// Option provided to repctl to generate output but skip applying.
	optDryRun = "--dry-run"

	// name of the environment variable used to provide a path to repctl executable.
	envRepctlPath = "REPCTL_PATH"
)

// relative location of the directory containing the config files fed to `repctl create sc` command.
var configFileDir = "testdata"

// getRepctlPath searches for a repctl executable and returns a file path to the executable.
// If the executable cannot be found, an error is returned.
// Search paths include csm-replication/repctl, the path provided by environment variable REPCTL_PATH,
// and PATH, in respective order.
func getRepctlPath() (path string, err error) {
	fmt.Println("looking for the repctl executable in csm-replication/repctl")

	// Try to locate repctl in the default build output location
	_, err = os.Stat("../../repctl")
	if err == nil {
		return "../../repctl", nil
	}
	fmt.Printf("error looking for repctl in csm-replication/repctl: %s\n", err.Error())

	// Try to locate repctl using the path provided via REPCTL_PATH
	fmt.Println("looking for repctl using the path provided via REPCTL_PATH")
	path = filepath.Join(os.Getenv(envRepctlPath), repctlCLI)
	_, err = os.Stat(path)
	if err == nil {
		return path, nil
	}
	fmt.Printf("error looking for repctl in REPCTL_PATH: %s\n", err.Error())

	// Try to locate repctl as part of one of the paths in PATH
	fmt.Println("looking for repctl in PATH")
	path, err = exec.LookPath(repctlCLI)
	if err == nil {
		return path, nil
	}
	fmt.Printf("error looking for repctl in PATH: %s\n", err.Error())

	return "", fmt.Errorf("unable to locate repctl. All methods were exhausted")
}

// use repctl to generate power scale storage class and compare
// the output to the expected output
func TestGeneratePowerScaleStorageClass(t *testing.T) {
	repctl, err := getRepctlPath()
	if err != nil {
		t.Errorf("encountered error while searching for repctl: %s", err.Error())
		t.SkipNow()
	}

	type args struct {
		templateName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "generate a csi-powerscale replication storage class with all options populated",
			args: args{
				templateName: "powerscale",
			},
			want: powerscaleStorageClass,
		},
	}

	for _, tt := range tests {
		configFilePath := filepath.Join(configFileDir, tt.args.templateName+"_tmpl_values.yaml")
		generated, err := exec.CommandContext(context.Background(), repctl, cmdCreate, resourceStorageClass, optFromConfig, configFilePath, optDryRun).CombinedOutput()
		if err != nil {
			t.Errorf("encountered error while generating storage class: %s", err.Error())
			t.FailNow()
		}

		// remove the log message printed by repctl at the beginning of the print out
		newlineIndex := bytes.IndexRune(generated, '\n')
		generated = generated[newlineIndex:]

		assert.Equal(t, string(tt.want), string(generated))
	}
}
