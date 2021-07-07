// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/onsi/gomega/gexec"
	contourv1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/pkg/config"
	"gopkg.in/yaml.v2"
	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	apimachinery_util_yaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Deployment struct {
	// k8s client
	client client.Client

	// Command output and other logs are written to this writer.
	logOutputWriter io.Writer

	// Fields for Local Contour deployment.

	// Path to kube config to use with a local Contour.
	kubeConfig string
	// Hostname to use when running Contour locally.
	localContourHost string
	// Port for local Contour to bind to.
	localContourPort string
	// Path to Contour binary for use when running locally.
	contourBin string

	// Fields for in-cluster Contour deployment

	// Contour image to use in in-cluster deployment.
	contourImage string

	Namespace                 *v1.Namespace
	ContourServiceAccount     *v1.ServiceAccount
	EnvoyServiceAccount       *v1.ServiceAccount
	ContourConfigMap          *v1.ConfigMap
	ExtensionServiceCRD       *apiextensions_v1.CustomResourceDefinition
	HTTPProxyCRD              *apiextensions_v1.CustomResourceDefinition
	TLSCertDelegationCRD      *apiextensions_v1.CustomResourceDefinition
	CertgenServiceAccount     *v1.ServiceAccount
	ContourRoleBinding        *rbac_v1.RoleBinding
	CertgenRole               *rbac_v1.Role
	CertgenJob                *batch_v1.Job
	ContourClusterRoleBinding *rbac_v1.ClusterRoleBinding
	ContourClusterRole        *rbac_v1.ClusterRole
	ContourService            *v1.Service
	EnvoyService              *v1.Service
	ContourDeployment         *apps_v1.Deployment
	EnvoyDaemonSet            *apps_v1.DaemonSet

	// Ratelimit deployment.
	RateLimitDeployment       *apps_v1.Deployment
	RateLimitService          *v1.Service
	RateLimitExtensionService *contourv1alpha1.ExtensionService
}

// UnmarshalResources unmarshals resources from rendered Contour manifest in
// order.
// Note: This will need to be updated if any new resources are added to the
// rendered deployment manifest.
func (d *Deployment) UnmarshalResources() error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("could not get path to this source file (test/e2e/deployment.go)")
	}
	renderedManifestPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "render", "contour.yaml")
	file, err := os.Open(renderedManifestPath)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := apimachinery_util_yaml.NewYAMLToJSONDecoder(file)

	// Discard empty document.
	if err := decoder.Decode(new(struct{})); err != nil {
		return err
	}

	d.Namespace = new(v1.Namespace)
	d.ContourServiceAccount = new(v1.ServiceAccount)
	d.EnvoyServiceAccount = new(v1.ServiceAccount)
	d.ContourConfigMap = new(v1.ConfigMap)
	d.ExtensionServiceCRD = new(apiextensions_v1.CustomResourceDefinition)
	d.HTTPProxyCRD = new(apiextensions_v1.CustomResourceDefinition)
	d.TLSCertDelegationCRD = new(apiextensions_v1.CustomResourceDefinition)
	d.CertgenServiceAccount = new(v1.ServiceAccount)
	d.ContourRoleBinding = new(rbac_v1.RoleBinding)
	d.CertgenRole = new(rbac_v1.Role)
	d.CertgenJob = new(batch_v1.Job)
	d.ContourClusterRoleBinding = new(rbac_v1.ClusterRoleBinding)
	d.ContourClusterRole = new(rbac_v1.ClusterRole)
	d.ContourService = new(v1.Service)
	d.EnvoyService = new(v1.Service)
	d.ContourDeployment = new(apps_v1.Deployment)
	d.EnvoyDaemonSet = new(apps_v1.DaemonSet)
	objects := []interface{}{
		d.Namespace,
		d.ContourServiceAccount,
		d.EnvoyServiceAccount,
		d.ContourConfigMap,
		d.ExtensionServiceCRD,
		d.HTTPProxyCRD,
		d.TLSCertDelegationCRD,
		d.CertgenServiceAccount,
		d.ContourRoleBinding,
		d.CertgenRole,
		d.CertgenJob,
		d.ContourClusterRoleBinding,
		d.ContourClusterRole,
		d.ContourService,
		d.EnvoyService,
		d.ContourDeployment,
		d.EnvoyDaemonSet,
	}
	for _, o := range objects {
		if err := decoder.Decode(o); err != nil {
			return err
		}
	}

	rateLimitExamplePath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "ratelimit")
	rateLimitDeploymentFile := filepath.Join(rateLimitExamplePath, "02-ratelimit.yaml")
	rateLimitExtSvcFile := filepath.Join(rateLimitExamplePath, "03-ratelimit-extsvc.yaml")

	rLDFile, err := os.Open(rateLimitDeploymentFile)
	if err != nil {
		return err
	}
	defer rLDFile.Close()
	decoder = apimachinery_util_yaml.NewYAMLToJSONDecoder(rLDFile)
	d.RateLimitDeployment = new(apps_v1.Deployment)
	if err := decoder.Decode(d.RateLimitDeployment); err != nil {
		return err
	}
	d.RateLimitService = new(v1.Service)
	if err := decoder.Decode(d.RateLimitService); err != nil {
		return err
	}

	rLESFile, err := os.Open(rateLimitExtSvcFile)
	if err != nil {
		return err
	}
	defer rLESFile.Close()
	decoder = apimachinery_util_yaml.NewYAMLToJSONDecoder(rLESFile)
	d.RateLimitExtensionService = new(contourv1alpha1.ExtensionService)
	if err := decoder.Decode(d.RateLimitExtensionService); err != nil {
		return err
	}

	return nil
}

// Common case of updating object if exists, create otherwise.
func (d *Deployment) ensureResource(new, existing client.Object) error {
	if err := d.client.Get(context.TODO(), client.ObjectKeyFromObject(new), existing); err != nil {
		if api_errors.IsNotFound(err) {
			return d.client.Create(context.TODO(), new)
		}
		return err
	}
	new.SetResourceVersion(existing.GetResourceVersion())
	// If a v1.Service, pass along existing cluster IP and healthcheck node port.
	if newS, ok := new.(*v1.Service); ok {
		existingS := existing.(*v1.Service)
		newS.Spec.ClusterIP = existingS.Spec.ClusterIP
		newS.Spec.ClusterIPs = existingS.Spec.ClusterIPs
		newS.Spec.HealthCheckNodePort = existingS.Spec.HealthCheckNodePort
	}
	return d.client.Update(context.TODO(), new)
}

func (d *Deployment) EnsureNamespace() error {
	return d.ensureResource(d.Namespace, new(v1.Namespace))
}

func (d *Deployment) EnsureContourServiceAccount() error {
	return d.ensureResource(d.ContourServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureEnvoyServiceAccount() error {
	return d.ensureResource(d.EnvoyServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureContourConfigMap() error {
	return d.ensureResource(d.ContourConfigMap, new(v1.ConfigMap))
}

func (d *Deployment) EnsureExtensionServiceCRD() error {
	return d.ensureResource(d.ExtensionServiceCRD, new(apiextensions_v1.CustomResourceDefinition))
}

func (d *Deployment) EnsureHTTPProxyCRD() error {
	return d.ensureResource(d.HTTPProxyCRD, new(apiextensions_v1.CustomResourceDefinition))
}

func (d *Deployment) EnsureTLSCertDelegationCRD() error {
	return d.ensureResource(d.TLSCertDelegationCRD, new(apiextensions_v1.CustomResourceDefinition))
}

func (d *Deployment) EnsureCertgenServiceAccount() error {
	return d.ensureResource(d.CertgenServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureContourRoleBinding() error {
	return d.ensureResource(d.ContourRoleBinding, new(rbac_v1.RoleBinding))
}

func (d *Deployment) EnsureCertgenRole() error {
	return d.ensureResource(d.CertgenRole, new(rbac_v1.Role))
}

func (d *Deployment) EnsureCertgenJob() error {
	// Delete job if exists with same name, then create.
	tempJ := new(batch_v1.Job)
	jobDeleted := func() (bool, error) {
		return api_errors.IsNotFound(d.client.Get(context.TODO(), client.ObjectKeyFromObject(d.CertgenJob), tempJ)), nil
	}
	if ok, _ := jobDeleted(); !ok {
		if err := d.client.Delete(context.TODO(), tempJ); err != nil {
			return err
		}
	}
	if err := wait.PollImmediate(time.Millisecond*50, time.Minute, jobDeleted); err != nil {
		return err
	}
	return d.client.Create(context.TODO(), d.CertgenJob)
}

func (d *Deployment) EnsureContourClusterRoleBinding() error {
	return d.ensureResource(d.ContourClusterRoleBinding, new(rbac_v1.ClusterRoleBinding))
}

func (d *Deployment) EnsureContourClusterRole() error {
	return d.ensureResource(d.ContourClusterRole, new(rbac_v1.ClusterRole))
}

func (d *Deployment) EnsureContourService() error {
	return d.ensureResource(d.ContourService, new(v1.Service))
}

func (d *Deployment) EnsureEnvoyService() error {
	return d.ensureResource(d.EnvoyService, new(v1.Service))
}

func (d *Deployment) EnsureContourDeployment() error {
	return d.ensureResource(d.ContourDeployment, new(apps_v1.Deployment))
}

func (d *Deployment) WaitForContourDeploymentUpdated() error {
	// List pods with app label "contour" and check that pods are updated
	// with expected container image and in ready state.
	// We do this instead of checking Deployment status as it is possible
	// for it not to have been updated yet and replicas not yet been shut
	// down.

	if len(d.ContourDeployment.Spec.Template.Spec.Containers) != 1 {
		return errors.New("invalid Contour Deployment containers spec")
	}
	contourPodImage := d.ContourDeployment.Spec.Template.Spec.Containers[0].Image
	updatedPods := func() (bool, error) {
		pods := new(v1.PodList)
		labelSelectAppContour := &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(d.ContourDeployment.Spec.Selector.MatchLabels),
			Namespace:     d.ContourDeployment.Namespace,
		}
		if err := d.client.List(context.TODO(), pods, labelSelectAppContour); err != nil {
			return false, err
		}
		if pods == nil {
			return false, errors.New("failed to fetch Contour Deployment pods")
		}

		updatedPods := 0
		for _, pod := range pods.Items {
			if len(pod.Spec.Containers) != 1 {
				return false, errors.New("invalid Contour Deployment pod containers")
			}
			if pod.Spec.Containers[0].Image != contourPodImage {
				continue
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
					updatedPods++
				}
			}
		}
		return updatedPods == int(*d.ContourDeployment.Spec.Replicas), nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute, updatedPods)
}

func (d *Deployment) EnsureEnvoyDaemonSet() error {
	return d.ensureResource(d.EnvoyDaemonSet, new(apps_v1.DaemonSet))
}

func (d *Deployment) WaitForEnvoyDaemonSetUpdated() error {
	daemonSetUpdated := func() (bool, error) {
		tempDS := new(apps_v1.DaemonSet)
		if err := d.client.Get(context.TODO(), client.ObjectKeyFromObject(d.EnvoyDaemonSet), tempDS); err != nil {
			return false, err
		}
		// This might work for now while we have only one worker node, but
		// if we expand to more, we will have to rethink this.
		return tempDS.Status.NumberReady > 0, nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute*3, daemonSetUpdated)
}

func (d *Deployment) EnsureResourcesForInclusterContour() error {
	fmt.Fprintln(d.logOutputWriter, "using contour image:", d.contourImage)

	fmt.Fprintln(d.logOutputWriter, "updating contour namespace")
	if err := d.EnsureNamespace(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour service account")
	if err := d.EnsureContourServiceAccount(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating envoy service account")
	if err := d.EnsureEnvoyServiceAccount(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour config map")
	if err := d.EnsureContourConfigMap(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour CRDs")
	if err := d.EnsureExtensionServiceCRD(); err != nil {
		return err
	}
	if err := d.EnsureHTTPProxyCRD(); err != nil {
		return err
	}
	if err := d.EnsureTLSCertDelegationCRD(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating certgen service account")
	if err := d.EnsureCertgenServiceAccount(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour role binding")
	if err := d.EnsureContourRoleBinding(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating certgen role")
	if err := d.EnsureCertgenRole(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating certgen job")
	// Update container image.
	if l := len(d.CertgenJob.Spec.Template.Spec.Containers); l != 1 {
		return fmt.Errorf("invalid certgen job containers, expected 1, got %d", l)
	}
	d.CertgenJob.Spec.Template.Spec.Containers[0].Image = d.contourImage
	d.CertgenJob.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	if err := d.EnsureCertgenJob(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour cluster role binding")
	if err := d.EnsureContourClusterRoleBinding(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour cluster role")
	if err := d.EnsureContourClusterRole(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour service")
	if err := d.EnsureContourService(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating envoy service")
	if err := d.EnsureEnvoyService(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating contour deployment")
	// Update container image.
	if l := len(d.ContourDeployment.Spec.Template.Spec.Containers); l != 1 {
		return fmt.Errorf("invalid contour deployment containers, expected 1, got %d", l)
	}
	d.ContourDeployment.Spec.Template.Spec.Containers[0].Image = d.contourImage
	d.ContourDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	if err := d.EnsureContourDeployment(); err != nil {
		return err
	}

	fmt.Fprintln(d.logOutputWriter, "updating envoy daemonset")
	// Update container image.
	if l := len(d.EnvoyDaemonSet.Spec.Template.Spec.InitContainers); l != 1 {
		return fmt.Errorf("invalid envoy daemonset init containers, expected 1, got %d", l)
	}
	d.EnvoyDaemonSet.Spec.Template.Spec.InitContainers[0].Image = d.contourImage
	d.EnvoyDaemonSet.Spec.Template.Spec.InitContainers[0].ImagePullPolicy = v1.PullIfNotPresent
	if l := len(d.EnvoyDaemonSet.Spec.Template.Spec.Containers); l != 2 {
		return fmt.Errorf("invalid envoy daemonset containers, expected 2, got %d", l)
	}
	d.EnvoyDaemonSet.Spec.Template.Spec.Containers[0].Image = d.contourImage
	d.EnvoyDaemonSet.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	d.EnvoyDaemonSet.Spec.Template.Spec.Containers[0].Args = append(d.EnvoyDaemonSet.Spec.Template.Spec.Containers[0].Args, "--check-delay=0s")
	if err := d.EnsureEnvoyDaemonSet(); err != nil {
		return err
	}

	return nil
}

func (d *Deployment) EnsureRateLimitResources(namespace string, configContents string) error {
	setNamespace := d.Namespace.Name
	if len(namespace) > 0 {
		setNamespace = namespace
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ratelimit-config",
			Namespace: setNamespace,
		},
		Data: map[string]string{
			"ratelimit-config.yaml": configContents,
		},
	}
	if err := d.ensureResource(configMap, new(v1.ConfigMap)); err != nil {
		return err
	}

	deployment := d.RateLimitDeployment.DeepCopy()
	deployment.Namespace = setNamespace
	if err := d.ensureResource(deployment, new(apps_v1.Deployment)); err != nil {
		return err
	}

	service := d.RateLimitService.DeepCopy()
	service.Namespace = setNamespace
	if err := d.ensureResource(service, new(v1.Service)); err != nil {
		return err
	}

	extSvc := d.RateLimitExtensionService.DeepCopy()
	extSvc.Namespace = setNamespace
	return d.ensureResource(extSvc, new(contourv1alpha1.ExtensionService))
}

// Convenience method for deploying the pieces of the deployment needed for
// testing Contour running locally, out of cluster.
// Includes:
// - namespace
// - Envoy service account
// - CRDs
// - Envoy service
// - ConfigMap with Envoy bootstrap config
// - Envoy DaemonSet modified for local Contour xDS server
func (d *Deployment) EnsureResourcesForLocalContour() error {
	if err := d.EnsureNamespace(); err != nil {
		return err
	}
	if err := d.EnsureEnvoyServiceAccount(); err != nil {
		return err
	}
	if err := d.EnsureExtensionServiceCRD(); err != nil {
		return err
	}
	if err := d.EnsureHTTPProxyCRD(); err != nil {
		return err
	}
	if err := d.EnsureTLSCertDelegationCRD(); err != nil {
		return err
	}
	if err := d.EnsureEnvoyService(); err != nil {
		return err
	}

	bFile, err := ioutil.TempFile("", "bootstrap-*.json")
	if err != nil {
		return err
	}

	// Generate bootstrap config with Contour local address and plaintext
	// client config.
	bootstrapCmd := exec.Command( // nolint:gosec
		d.contourBin,
		"bootstrap",
		bFile.Name(),
		"--xds-address="+d.localContourHost,
		"--xds-port="+d.localContourPort,
		"--xds-resource-version=v3",
	)
	session, err := gexec.Start(bootstrapCmd, d.logOutputWriter, d.logOutputWriter)
	if err != nil {
		return err
	}
	session.Wait()

	bootstrapContents, err := ioutil.ReadAll(bFile)
	if err != nil {
		return err
	}
	defer func() {
		bFile.Close()
		os.RemoveAll(bFile.Name())
	}()

	bootstrapConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envoy-bootstrap",
			Namespace: d.Namespace.Name,
		},
		Data: map[string]string{
			"envoy.json": string(bootstrapContents),
		},
	}
	if err := d.ensureResource(bootstrapConfigMap, new(v1.ConfigMap)); err != nil {
		return err
	}

	// Add bootstrap ConfigMap as volume on Envoy pods (also removes cert volume).
	d.EnvoyDaemonSet.Spec.Template.Spec.Volumes = []v1.Volume{
		{
			Name: "envoy-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: "envoy-bootstrap",
					},
				},
			},
		},
	}

	// Remove cert volume mount.
	d.EnvoyDaemonSet.Spec.Template.Spec.Containers[1].VolumeMounts = []v1.VolumeMount{d.EnvoyDaemonSet.Spec.Template.Spec.Containers[1].VolumeMounts[0]}

	// Remove init container.
	d.EnvoyDaemonSet.Spec.Template.Spec.InitContainers = nil
	// Remove shutdown-manager container.
	d.EnvoyDaemonSet.Spec.Template.Spec.Containers = d.EnvoyDaemonSet.Spec.Template.Spec.Containers[1:]

	if err := d.EnsureEnvoyDaemonSet(); err != nil {
		return err
	}

	return nil
}

// Starts local contour, applying arguments and marshaling config into config
// file. Returns running Contour command and config file so we can clean them
// up.
func (d *Deployment) StartLocalContour(config *config.Parameters, additionalArgs ...string) (*gexec.Session, string, error) {
	configFile, err := ioutil.TempFile("", "contour-config-*.yaml")
	if err != nil {
		return nil, "", err
	}
	defer configFile.Close()

	content, err := yaml.Marshal(config)
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(configFile.Name(), content, 0644); err != nil {
		return nil, "", err
	}

	contourServeArgs := append([]string{
		"serve",
		"--xds-address=0.0.0.0",
		"--xds-port=" + d.localContourPort,
		"--insecure",
		"--kubeconfig=" + d.kubeConfig,
		"--config-path=" + configFile.Name(),
		"--disable-leader-election",
	}, additionalArgs...)
	session, err := gexec.Start(exec.Command(d.contourBin, contourServeArgs...), d.logOutputWriter, d.logOutputWriter) // nolint:gosec
	if err != nil {
		return nil, "", err
	}
	return session, configFile.Name(), nil
}

func (d *Deployment) StopLocalContour(contourCmd *gexec.Session, configFile string) error {
	contourCmd.Terminate().Wait()
	return os.RemoveAll(configFile)
}
