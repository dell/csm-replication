module github.com/dell/csm-replication

go 1.16

require (
	github.com/dell/dell-csi-extensions/replication v1.0.0
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
	golang.org/x/net v0.0.0-20210428140749-89ef3d95e781
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/grpc v1.38.0
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.1
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/kustomize/kustomize/v3 v3.10.0
)
