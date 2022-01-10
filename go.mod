module github.com/dell/csm-replication

go 1.16

require (
	github.com/dell/dell-csi-extensions/replication v1.0.0
	github.com/dell/dell-csi-extensions/common v1.0.0
	github.com/fatih/color v1.7.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-chi/chi v1.5.1
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.1.2
	github.com/lithammer/fuzzysearch v1.1.1
	github.com/maxan98/logrusr v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/viper v1.7.0
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/grpc v1.42.0
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	sigs.k8s.io/controller-runtime v0.9.1
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/kustomize/kustomize/v3 v3.10.0
)

replace (
	github.com/dell/dell-csi-extensions/common v1.0.0 => github.com/dell/dell-csi-extensions/common v0.0.0-20211217121714-58de430139aa
	github.com/dell/dell-csi-extensions/replication v1.0.0 => github.com/dell/dell-csi-extensions/replication v0.0.0-20211217121714-58de430139aa
)
