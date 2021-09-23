module github.com/tencent/lighthouse-plugin

go 1.14

require (
	github.com/Microsoft/hcsshim v0.8.14 // indirect
	github.com/containerd/cgroups v0.0.0-20201119153540-4cbc285b3327 // indirect
	github.com/containerd/containerd v1.4.3 // indirect
	github.com/containerd/continuity v0.0.0-20201208142359-180525291bb7 // indirect
	github.com/coreos/go-systemd/v22 v22.1.0
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v20.10.1+incompatible
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/evanphx/json-patch v4.2.0+incompatible
	github.com/gorilla/mux v1.7.0
	github.com/json-iterator/go v1.1.8
	github.com/mYmNeo/version v0.0.0-20200424030557-30e59e77cc3e
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	gotest.tools/v3 v3.0.3 // indirect
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/apiserver v0.17.4
	k8s.io/client-go v0.17.4
	k8s.io/component-base v0.17.4
	k8s.io/klog v1.0.0
)

replace (
	github.com/Sirupsen/logrus v1.7.0 => github.com/sirupsen/logrus v1.7.0
	github.com/sirupsen/logrus v1.7.0 => github.com/Sirupsen/logrus v1.7.0
)
