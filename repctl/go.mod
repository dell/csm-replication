module github.com/dell/repctl

go 1.16

replace github.com/dell/csm-replication => ../

require (
	github.com/dell/csm-replication v1.1.0
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/rifflock/lfshook v0.0.0-20180920164130-b9218ef580f5
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.3.0
	github.com/spf13/viper v1.10.1
	github.com/stretchr/testify v1.7.0
	github.com/vektra/mockery/v2 v2.10.0
	github.com/x-cray/logrus-prefixed-formatter v0.5.2
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.23.0-alpha.1
	k8s.io/apiextensions-apiserver v0.23.0-alpha.1
	k8s.io/apimachinery v0.23.0-alpha.1
	k8s.io/client-go v0.23.0-alpha.1
	sigs.k8s.io/controller-runtime v0.10.3
)
