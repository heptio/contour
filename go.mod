module github.com/projectcontour/contour

go 1.15

require (
	github.com/ahmetb/gen-crd-api-reference-docs v0.3.0
	github.com/bombsimon/logrusr v1.0.0
	github.com/envoyproxy/go-control-plane v0.9.10-0.20210614203518-782de910ff04
	github.com/go-logr/logr v0.4.0
	github.com/golang/protobuf v1.5.0
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.1.2
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/jetstack/cert-manager v1.3.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.10.5
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.15.0
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.7.0
	google.golang.org/genproto v0.0.0-20201110150050-8816d57aaa9a
	google.golang.org/grpc v1.36.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.0
	k8s.io/apiextensions-apiserver v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.21.0
	k8s.io/klog/v2 v2.8.1-0.20210504170414-0cc9b8363efc
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10
	sigs.k8s.io/controller-runtime v0.9.0-beta.1
	sigs.k8s.io/controller-tools v0.5.0
	sigs.k8s.io/gateway-api v0.3.0
	sigs.k8s.io/kustomize/kyaml v0.1.1
)
