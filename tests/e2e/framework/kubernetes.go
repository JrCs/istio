// Copyright 2017 Istio Authors
//
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

package framework

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	testKube "istio.io/istio/pkg/test/kube"
	"istio.io/istio/pkg/test/util/retry"
	"istio.io/istio/tests/util"
	"istio.io/pkg/log"
)

const (
	yamlSuffix                     = ".yaml"
	istioInstallDir                = "install/kubernetes"
	initInstallFile                = "istio-init.yaml"
	nonAuthInstallFile             = "istio.yaml"
	authInstallFile                = "istio-auth.yaml"
	authSdsInstallFile             = "istio-auth-sds.yaml"
	nonAuthWithoutMCPInstallFile   = "istio-mcp.yaml"
	authWithoutMCPInstallFile      = "istio-auth-mcp.yaml"
	nonAuthInstallFileNamespace    = "istio-one-namespace.yaml"
	authInstallFileNamespace       = "istio-one-namespace-auth.yaml"
	trustDomainFileNamespace       = "istio-one-namespace-trust-domain.yaml"
	mcNonAuthInstallFileNamespace  = "istio-multicluster.yaml"
	mcAuthInstallFileNamespace     = "istio-auth-multicluster.yaml"
	mcRemoteInstallFile            = "istio-remote.yaml"
	mcSplitHorizonInstallFile      = "istio-multicluster-split-horizon.yaml"
	authOperatorInstallFile        = "istio-operator.yaml"
	istioSystem                    = "istio-system"
	istioIngressServiceName        = "istio-ingress"
	istioIngressLabel              = "ingress"
	istioIngressGatewayServiceName = "istio-ingressgateway"
	istioIngressGatewayLabel       = "ingressgateway"
	istioEgressGatewayServiceName  = "istio-egressgateway"
	defaultSidecarInjectorFile     = "istio-sidecar-injector.yaml"
	ingressCertsName               = "istio-ingress-certs"
	maxDeploymentRolloutTime       = 960 * time.Second
	maxValidationReadyCheckTime    = 120 * time.Second
	maxCNIDeployTime               = 10 * time.Second
	helmServiceAccountFile         = "helm-service-account.yaml"
	istioHelmInstallDir            = istioInstallDir + "/helm/istio"
	caCertFileName                 = "samples/certs/ca-cert.pem"
	caKeyFileName                  = "samples/certs/ca-key.pem"
	rootCertFileName               = "samples/certs/root-cert.pem"
	certChainFileName              = "samples/certs/cert-chain.pem"
	helmInstallerName              = "helm"
	remoteNetworkName              = "network2"
	istioHelmChartName             = "istio"
	// CRD files that should be installed during testing
	// NB: these files come from the directory install/kubernetes/helm/istio-init/files/*crd*
	//     and contain all CRDs used by Istio during runtime
	allCRDInstallFile   = "crd-all.gen.yaml"
	mixerCRDInstallFile = "crd-mixer.yaml"
	// PrimaryCluster identifies the primary cluster
	PrimaryCluster = "primary"
	// RemoteCluster identifies the remote cluster
	RemoteCluster = "remote"

	kubernetesReadinessTimeout        = time.Second * 180
	kubernetesReadinessInterval       = 200 * time.Millisecond
	validationWebhookReadinessTimeout = time.Minute
	istioOperatorTimeout              = time.Second * 300
	istioOperatorFreq                 = time.Second * 10
	validationWebhookReadinessFreq    = 100 * time.Millisecond
)

var (
	namespace          = flag.String("namespace", "", "Namespace to use for testing (empty to create/delete temporary one)")
	istioNamespace     = flag.String("istio_namespace", "istio-system", "Istio system namespace (default `istio-system`)")
	mixerHub           = flag.String("mixer_hub", os.Getenv("HUB"), "Mixer hub")
	mixerTag           = flag.String("mixer_tag", os.Getenv("TAG"), "Mixer tag")
	pilotHub           = flag.String("pilot_hub", os.Getenv("HUB"), "Pilot hub")
	pilotTag           = flag.String("pilot_tag", os.Getenv("TAG"), "Pilot tag")
	appHub             = flag.String("app_hub", os.Getenv("HUB"), "Test application hub")
	appTag             = flag.String("app_tag", os.Getenv("TAG"), "Test application tag")
	proxyHub           = flag.String("proxy_hub", os.Getenv("HUB"), "Proxy hub")
	proxyTag           = flag.String("proxy_tag", os.Getenv("TAG"), "Proxy tag")
	caHub              = flag.String("ca_hub", os.Getenv("HUB"), "Ca hub")
	caTag              = flag.String("ca_tag", os.Getenv("TAG"), "Ca tag")
	galleyHub          = flag.String("galley_hub", os.Getenv("HUB"), "Galley hub")
	galleyTag          = flag.String("galley_tag", os.Getenv("TAG"), "Galley tag")
	cniHub             = flag.String("cni_hub", os.Getenv("ISTIO_CNI_HUB"), "CNI hub")
	cniTag             = flag.String("cni_tag", os.Getenv("ISTIO_CNI_TAG"), "CNI tag")
	sidecarInjectorHub = flag.String("sidecar_injector_hub", os.Getenv("HUB"), "Sidecar injector hub")
	sidecarInjectorTag = flag.String("sidecar_injector_tag", os.Getenv("TAG"), "Sidecar injector tag")
	trustDomainEnable  = flag.Bool("trust_domain_enable", false, "Enable different trust domains (e.g. test.local)")
	authEnable         = flag.Bool("auth_enable", false, "Enable auth")
	authSdsEnable      = flag.Bool("auth_sds_enable", false, "Enable auth using key/cert distributed through SDS")
	rbacEnable         = flag.Bool("rbac_enable", true, "Enable rbac")
	localCluster       = flag.Bool("use_local_cluster", false,
		"If true any LoadBalancer type services will be converted to a NodePort service during testing. If running on minikube, this should be set to true")
	skipSetup           = flag.Bool("skip_setup", false, "Skip namespace creation and istio cluster setup")
	sidecarInjectorFile = flag.String("sidecar_injector_file", defaultSidecarInjectorFile, "Sidecar injector yaml file")
	clusterWide         = flag.Bool("cluster_wide", false, "If true Pilot/Mixer will observe all namespaces rather than just the testing namespace")
	imagePullPolicy     = flag.String("image_pull_policy", "", "Specifies an override for the Docker image pull policy to be used")
	multiClusterDir     = flag.String("cluster_registry_dir", "",
		"Directory name for the cluster registry config. When provided a multicluster test is run across two clusters.")
	splitHorizon             = flag.Bool("split_horizon", false, "Set up a split horizon EDS multi-cluster test environment")
	useGalleyConfigValidator = flag.Bool("use_galley_config_validator", true, "Use galley configuration validation webhook")
	installer                = flag.String("installer", "kubectl", "Istio installer, default to kubectl, or helm")
	useMCP                   = flag.Bool("use_mcp", true, "use MCP for configuring Istio components")
	useOperator              = flag.Bool("use_operator", false, "use Operator to deploy Istio components")
	outboundTrafficPolicy    = flag.String("outbound_trafficpolicy", "ALLOW_ANY", "Istio outbound traffic policy, default to ALLOW_ANY")
	enableEgressGateway      = flag.Bool("enable_egressgateway", false, "enable egress gateway, default to false")
	useCNI                   = flag.Bool("use_cni", false,
		"Install the Istio CNI which will add the IP table rules for Envoy instead of the init container")
	cniHelmRepo  = flag.String("cni_helm_repo", "istio.io/istio-cni", "Name of the Istio CNI helm repo")
	kubeInjectCM = flag.String("kube_inject_configmap", "",
		"Configmap to use by the istioctl kube-inject command.")
	valueFile     = flag.String("valueFile", "", "Istio value yaml file when helm is used")
	helmSetValues helmSetValueList
)

// Support for multiple values for helm installation
type helmSetValueList []string

func (h *helmSetValueList) String() string {
	return fmt.Sprintf("%v", *h)
}
func (h *helmSetValueList) Set(value string) error {
	if len(*h) > 0 {
		return errors.New("helmSetValueList flag already set")
	}
	for _, v := range strings.Split(value, ",") {
		*h = append(*h, v)
	}
	return nil
}
func init() {
	flag.Var(&helmSetValues, "helmSetValueList", "Additional helm values parsed, eg: galley.enabled=true,global.useMCP=true")
}

type appPodsInfo struct {
	// A map of app label values to the pods for that app
	Pods      map[string][]string
	PodsMutex sync.Mutex
}

// KubeInfo gathers information for kubectl
type KubeInfo struct {
	Namespace string

	TmpDir  string
	yamlDir string

	inglock    sync.Mutex
	ingress    string
	ingressErr error

	ingressGatewayLock sync.Mutex
	ingressGateway     string
	ingressGatewayErr  error

	localCluster     bool
	namespaceCreated bool
	AuthEnabled      bool
	AuthSdsEnabled   bool
	RBACEnabled      bool

	// Istioctl installation
	Istioctl *Istioctl
	// App Manager
	AppManager *AppManager

	// Release directory
	ReleaseDir string
	// Use baseversion if not empty.
	BaseVersion string

	appPods  map[string]*appPodsInfo
	Clusters map[string]string

	KubeConfig         string
	KubeAccessor       *testKube.Accessor
	RemoteKubeConfig   string
	RemoteKubeAccessor *testKube.Accessor
	RemoteAppManager   *AppManager
	RemoteIstioctl     *Istioctl
}

func getClusterWideInstallFile() string {
	var istioYaml string
	if *authEnable {
		if *useMCP {
			if *authSdsEnable {
				istioYaml = authSdsInstallFile
			} else if *useOperator {
				istioYaml = authOperatorInstallFile
			} else {
				istioYaml = authInstallFile
			}
		} else {
			istioYaml = authWithoutMCPInstallFile
		}
	} else {
		if *useMCP {
			istioYaml = nonAuthInstallFile
		} else {
			istioYaml = nonAuthWithoutMCPInstallFile
		}
	}
	if *trustDomainEnable {
		istioYaml = trustDomainFileNamespace
	}
	return istioYaml
}

// newKubeInfo create a new KubeInfo by given temp dir and runID
// If baseVersion is not empty, will use the specified release of Istio instead of the local one.
func newKubeInfo(tmpDir, runID, baseVersion string) (*KubeInfo, error) {
	if *namespace == "" {
		if *clusterWide {
			*namespace = istioSystem
		} else {
			*namespace = runID
		}
	}
	yamlDir := filepath.Join(tmpDir, "yaml")
	i, err := NewIstioctl(yamlDir, *namespace, *istioNamespace, *kubeInjectCM, "")
	if err != nil {
		return nil, err
	}

	// Download the base release if baseVersion is specified.
	var releaseDir string
	if baseVersion != "" {
		releaseDir, err = util.DownloadRelease(baseVersion, tmpDir)
		if err != nil {
			return nil, err
		}
		// Use istioctl from base version to inject the sidecar.
		i.localPath = filepath.Join(releaseDir, "/bin/istioctl")
		if err = os.Chmod(i.localPath, 0755); err != nil {
			return nil, err
		}
	} else {
		releaseDir = util.GetResourcePath("")
	}
	// Note the kubectl commands used by the test will default to use the local
	// environment's kubeconfig if an empty string is provided.  Therefore in the
	// default case kubeConfig will not be set.
	var kubeConfig, remoteKubeConfig string
	var remoteKubeAccessor *testKube.Accessor
	var aRemote *AppManager
	var remoteI *Istioctl
	if *multiClusterDir != "" {
		// multiClusterDir indicates the Kubernetes cluster config should come from files versus
		// the in cluster config. At the current time only the remote kubeconfig is read from a
		// file.
		tmpfile := *namespace + "_kubeconfig"
		tmpfile = path.Join(tmpDir, tmpfile)
		if err = util.GetKubeConfig(tmpfile); err != nil {
			return nil, err
		}
		kubeConfig = tmpfile
		remoteKubeConfig, err = getKubeConfigFromFile(*multiClusterDir)
		log.Infof("Remote kubeconfig file %s", remoteKubeConfig)
		if err != nil {
			// TODO could change this to continue tests if only a single cluster is in play
			return nil, err
		}
		if remoteKubeAccessor, err = testKube.NewAccessor(remoteKubeConfig, tmpDir); err != nil {
			return nil, err
		}
		// Create Istioctl for remote using injectConfigMap on remote (not the same as master cluster's)
		remoteI, err = NewIstioctl(yamlDir, *namespace, *istioNamespace, "istio-sidecar-injector", remoteKubeConfig)
		if err != nil {
			return nil, err
		}
		if baseVersion != "" {
			remoteI.localPath = filepath.Join(releaseDir, "/bin/istioctl")
		}
		if *splitHorizon {
			// For remote Istio configuration in a split horizon test we don't wait
			// for the deployments because they won't be able to connect to Pilot
			// until their service is added by watching the remote k8s api. Adding
			// the remote k8s api watcher is done later on in the test.
			aRemote = NewAppManager(tmpDir, *namespace, remoteI, remoteKubeConfig, false)
		} else {
			aRemote = NewAppManager(tmpDir, *namespace, remoteI, remoteKubeConfig, true)
		}
	}

	a := NewAppManager(tmpDir, *namespace, i, kubeConfig, true)

	kubeAccessor, err := testKube.NewAccessor(kubeConfig, tmpDir)
	if err != nil {
		return nil, err
	}

	clusters := make(map[string]string)
	appPods := make(map[string]*appPodsInfo)
	clusters[PrimaryCluster] = kubeConfig
	appPods[PrimaryCluster] = &appPodsInfo{}
	if remoteKubeConfig != "" {
		clusters[RemoteCluster] = remoteKubeConfig
		appPods[RemoteCluster] = &appPodsInfo{}
	}

	log.Infof("Using release dir: %s", releaseDir)
	return &KubeInfo{
		Namespace:          *namespace,
		namespaceCreated:   false,
		TmpDir:             tmpDir,
		yamlDir:            yamlDir,
		localCluster:       *localCluster,
		Istioctl:           i,
		RemoteIstioctl:     remoteI,
		AppManager:         a,
		RemoteAppManager:   aRemote,
		AuthEnabled:        *authEnable,
		AuthSdsEnabled:     *authSdsEnable,
		RBACEnabled:        *rbacEnable,
		ReleaseDir:         releaseDir,
		BaseVersion:        baseVersion,
		KubeConfig:         kubeConfig,
		KubeAccessor:       kubeAccessor,
		RemoteKubeConfig:   remoteKubeConfig,
		RemoteKubeAccessor: remoteKubeAccessor,
		appPods:            appPods,
		Clusters:           clusters,
	}, nil
}

// IsClusterWide indicates whether or not the environment is configured for a cluster-wide deployment.
func (k *KubeInfo) IsClusterWide() bool {
	return *clusterWide
}

// IstioSystemNamespace returns the namespace used for the Istio system components.
func (k *KubeInfo) IstioSystemNamespace() string {
	if *clusterWide {
		return istioSystem
	}
	return k.Namespace
}

// IstioIngressService returns the service name for the ingress service
func (k *KubeInfo) IstioIngressService() string {
	return istioIngressServiceName
}

// IstioIngressGatewayService returns the service name for the ingress gateway service
func (k *KubeInfo) IstioIngressGatewayService() string {
	return istioIngressGatewayServiceName
}

// IstioEgressGatewayService returns the service name for the egress gateway service
func (k *KubeInfo) IstioEgressGatewayService() string {
	return istioEgressGatewayServiceName
}

// Setup Kubernetes pre-requisites for tests
func (k *KubeInfo) Setup() error {
	log.Infoa("Setting up kubeInfo setupSkip=", *skipSetup)
	var err error
	if err = os.Mkdir(k.yamlDir, os.ModeDir|os.ModePerm); err != nil {
		return err
	}

	if err = k.waitForKubernetes(); err != nil {
		return err
	}

	if !*skipSetup {
		if *installer == helmInstallerName {
			// install helm tiller first
			yamlDir := filepath.Join(istioInstallDir+"/"+helmInstallerName, helmServiceAccountFile)
			baseHelmServiceAccountYaml := filepath.Join(k.ReleaseDir, yamlDir)
			if err = k.deployTiller(baseHelmServiceAccountYaml); err != nil {
				log.Error("Failed to deploy helm tiller.")
				return err
			}

			// install istio using helm
			if err = k.deployIstioWithHelm(); err != nil {
				log.Error("Failed to deploy Istio with helm.")
				return err
			}

			// execute helm test for istio
			if err = k.executeHelmTest(); err != nil {
				log.Error("Failed to execute Istio helm tests.")
				return err
			}
		} else if err = k.deployIstio(); err != nil {
			log.Error("Failed to deploy Istio.")
			return err
		}
		// Create the ingress secret.
		certDir := util.GetResourcePath("./tests/testdata/certs")
		certFile := filepath.Join(certDir, "cert.crt")
		keyFile := filepath.Join(certDir, "cert.key")
		if _, err = util.CreateTLSSecret(ingressCertsName, k.IstioSystemNamespace(), keyFile, certFile, k.KubeConfig); err != nil {
			log.Warn("Secret already exists")
		}
	}

	return nil
}

// PilotHub exposes the Docker hub used for the pilot image.
func (k *KubeInfo) PilotHub() string {
	return *pilotHub
}

// PilotTag exposes the Docker tag used for the pilot image.
func (k *KubeInfo) PilotTag() string {
	return *pilotTag
}

// AppHub exposes the Docker hub used for the test application image.
func (k *KubeInfo) AppHub() string {
	return *appHub
}

// AppTag exposes the Docker tag used for the test application image.
func (k *KubeInfo) AppTag() string {
	return *appTag
}

// ProxyHub exposes the Docker hub used for the proxy image.
func (k *KubeInfo) ProxyHub() string {
	return *proxyHub
}

// ProxyTag exposes the Docker tag used for the proxy image.
func (k *KubeInfo) ProxyTag() string {
	return *proxyTag
}

// ImagePullPolicy exposes the pull policy override used for Docker images. May be "".
func (k *KubeInfo) ImagePullPolicy() string {
	return *imagePullPolicy
}

// IngressOrFail lazily initialize ingress and fail test if not found.
func (k *KubeInfo) IngressOrFail(t *testing.T) string {
	gw, err := k.Ingress()
	if err != nil {
		t.Fatalf("Unable to get ingress: %v", err)
	}
	return gw
}

// Ingress lazily initialize ingress
func (k *KubeInfo) Ingress() (string, error) {
	return k.doGetIngress(istioIngressServiceName, istioIngressLabel, &k.inglock, &k.ingress, &k.ingressErr)
}

// IngressGatewayOrFail lazily initialize ingress gateway and fail test if not found.
func (k *KubeInfo) IngressGatewayOrFail(t *testing.T) string {
	gw, err := k.IngressGateway()
	if err != nil {
		t.Fatalf("Unable to get ingress: %v", err)
	}
	return gw
}

// IngressGateway lazily initialize Ingress Gateway
func (k *KubeInfo) IngressGateway() (string, error) {
	return k.doGetIngress(istioIngressGatewayServiceName, istioIngressGatewayLabel,
		&k.ingressGatewayLock, &k.ingressGateway, &k.ingressGatewayErr)
}

func (k *KubeInfo) doGetIngress(serviceName string, podLabel string, lock sync.Locker,
	ingress *string, ingressErr *error) (string, error) {
	lock.Lock()
	defer lock.Unlock()

	// Previously fetched ingress or failed.
	if *ingressErr != nil || len(*ingress) != 0 {
		return *ingress, *ingressErr
	}
	if k.localCluster {
		*ingress, *ingressErr = util.GetIngress(serviceName, podLabel,
			k.Istioctl.istioNamespace, k.KubeConfig, util.NodePortServiceType, true)
	} else {
		*ingress, *ingressErr = util.GetIngress(serviceName, podLabel,
			k.Istioctl.istioNamespace, k.KubeConfig, util.LoadBalancerServiceType, true)
	}

	// So far we only do http ingress
	if len(*ingress) > 0 {
		*ingress = "http://" + *ingress
	}

	return *ingress, *ingressErr
}

// Teardown clean up everything created by setup
func (k *KubeInfo) Teardown() error {
	log.Info("Cleaning up kubeInfo")

	if *skipSetup || *skipCleanup || os.Getenv("SKIP_CLEANUP") != "" {
		return nil
	}
	var errs error
	if *installer == helmInstallerName {
		// clean up using helm
		err := util.HelmDelete(istioHelmChartName)
		if err != nil {
			// If fail don't return so other cleanup activities can complete
			errs = multierror.Append(errs, multierror.Prefix(err, fmt.Sprintf("Could not delete %s", istioHelmChartName)))
		}

		if *useCNI {
			err := util.HelmDelete("istio-cni")
			if err != nil {
				// If fail don't return so other cleanup activities can complete
				errs = multierror.Append(errs, multierror.Prefix(err, fmt.Sprintf("Helm delete of chart %s failed", "istio-cni")))
			}
		}

		if err := util.DeleteNamespace(k.Namespace, k.KubeConfig); err != nil {
			errs = multierror.Append(errs, multierror.Prefix(err, fmt.Sprintf("Failed to delete namespace %s", k.Namespace)))
		}
	} else {
		if *useAutomaticInjection {
			testSidecarInjectorYAML := filepath.Join(k.TmpDir, "yaml", *sidecarInjectorFile)

			if err := util.KubeDelete(k.Namespace, testSidecarInjectorYAML, k.KubeConfig); err != nil {
				log.Errorf("Istio sidecar injector %s deletion failed", testSidecarInjectorYAML)
				return err
			}
		}

		var istioYaml string
		if *clusterWide {
			if *multiClusterDir != "" {
				if *authEnable {
					istioYaml = mcAuthInstallFileNamespace
				} else {
					istioYaml = mcNonAuthInstallFileNamespace
				}
			} else {
				istioYaml = getClusterWideInstallFile()
			}
		}
		if *useOperator {
			//save operator logs
			log.Info("Saving istio-operator logs")
			if err := util.FetchAndSaveClusterLogs("istio-operator", k.TmpDir, k.KubeConfig); err != nil {
				log.Errorf("Failed to save operator logs: %v", err)
			}
			// Need an operator unique delete procedure
			if _, err := util.Shell("kubectl -n istio-operator delete IstioOperator example-istiocontrolplane"); err != nil {
				log.Errorf("Failed to delete the Istio CR.")
				return err
			}
			if _, err := util.Shell("kubectl delete ns istio-operator --kubeconfig=%s",
				k.KubeConfig); err != nil {
				log.Errorf("Failed to delete istio-operator namespace.")
				return err
			}
			if _, err := util.Shell("kubectl delete ns %s --kubeconfig=%s",
				k.Namespace, k.KubeConfig); err != nil {
				log.Errorf("Failed to delete %s namespace.", k.Namespace)
				return err
			}
		} else {
			testIstioYaml := filepath.Join(k.TmpDir, "yaml", istioYaml)
			if err := util.KubeDelete(k.Namespace, testIstioYaml, k.KubeConfig); err != nil {
				log.Infof("Safe to ignore resource not found errors in kubectl delete -f %s", testIstioYaml)
			}

			if err := util.DeleteNamespace(k.Namespace, k.KubeConfig); err != nil {
				log.Errorf("Failed to delete namespace %s", k.Namespace)
				return err
			}

			// ClusterRoleBindings are not namespaced and need to be deleted separately
			if _, err := util.Shell("kubectl get --kubeconfig=%s clusterrolebinding -o jsonpath={.items[*].metadata.name}"+
				"|xargs -n 1|fgrep %s|xargs kubectl delete --kubeconfig=%s clusterrolebinding", k.KubeConfig,
				k.Namespace, k.KubeConfig); err != nil {
				log.Errorf("Failed to delete clusterrolebindings associated with namespace %s", k.Namespace)
				return err
			}

			// ClusterRoles are not namespaced and need to be deleted separately
			if _, err := util.Shell("kubectl get --kubeconfig=%s clusterrole -o jsonpath={.items[*].metadata.name}"+
				"|xargs -n 1|fgrep %s|xargs kubectl delete --kubeconfig=%s clusterrole", k.KubeConfig,
				k.Namespace, k.KubeConfig); err != nil {
				log.Errorf("Failed to delete clusterroles associated with namespace %s", k.Namespace)
				return err
			}
		}
	}
	if *multiClusterDir != "" {
		if err := util.DeleteNamespace(k.Namespace, k.RemoteKubeConfig); err != nil {
			errs = multierror.Append(errs, multierror.Prefix(err, fmt.Sprintf("Failed to delete namespace %s on remote cluster", k.Namespace)))
		}
	}

	// NB: Increasing maxAttempts much past 230 seconds causes the CI infrastructure
	// to terminate the test run not reporting what actually failed in the deletion.
	maxAttempts := 180

	namespaceDeleted := false
	validatingWebhookConfigurationExists := false
	log.Infof("Deleting namespace %v", k.Namespace)
	for attempts := 1; attempts <= maxAttempts; attempts++ {
		if *useOperator {
			namespaceDeleted, _ = util.NamespaceDeleted("istio-operator", k.KubeConfig)
		} else {
			namespaceDeleted, _ = util.NamespaceDeleted(k.Namespace, k.KubeConfig)
		}
		// As validatingWebhookConfiguration "istio-galley" will
		// be delete by kubernetes GC controller asynchronously,
		// we need to ensure it's deleted before return.
		// TODO: find a more general way as long term solution.
		validatingWebhookConfigurationExists = util.ValidatingWebhookConfigurationExists("istio-galley", k.KubeConfig)

		if namespaceDeleted && !validatingWebhookConfigurationExists {
			break
		}

		time.Sleep(1 * time.Second)
	}

	if !namespaceDeleted {
		log.Errorf("Failed to delete namespace %s after %v seconds", k.Namespace, maxAttempts)
		return nil
	}

	if validatingWebhookConfigurationExists {
		log.Errorf("Failed to delete validatingwebhookconfiguration istio-galley after %d seconds", maxAttempts)
		return nil
	}

	log.Infof("Namespace %s deletion status: %v", k.Namespace, namespaceDeleted)

	return errs
}

// GetAppPods gets a map of app name to pods for that app. If pods are found, the results are cached.
func (k *KubeInfo) GetAppPods(cluster string) map[string][]string {
	// Get a copy of the internal map.
	newMap := k.getAppPods(cluster)

	if len(newMap) == 0 {
		var err error
		if newMap, err = util.GetAppPods(k.Namespace, k.Clusters[cluster]); err != nil {
			log.Errorf("Failed to get retrieve the app pods for namespace %s", k.Namespace)
		} else {
			// Copy the new results to the internal map.
			log.Infof("Fetched pods with the `app` label: %v", newMap)
			k.setAppPods(cluster, newMap)
		}
	}
	return newMap
}

// CheckJobSucceeded checks whether the job succeeded.
func (k *KubeInfo) CheckJobSucceeded(cluster, jobName string) error {
	retry := util.Retrier{
		BaseDelay: 1 * time.Second,
		MaxDelay:  1 * time.Second,
		Retries:   15,
	}

	retryFn := func(_ context.Context, i int) error {
		ret, err := util.IsJobSucceeded(k.Namespace, jobName, k.Clusters[cluster])
		if err != nil {
			log.Errorf("Failed to get retrieve the app pods for namespace %s", k.Namespace)
			return err
		}
		if !ret {
			return fmt.Errorf("job %s not succeeded", jobName)
		}
		return nil
	}
	ctx := context.Background()
	_, err := retry.Retry(ctx, retryFn)
	if err != nil {
		return err
	}
	return nil
}

// GetRoutes gets routes from the pod or returns error
func (k *KubeInfo) GetRoutes(app string) (routes string, err error) {
	routesURL := "http://localhost:15000/config_dump"
	for cluster := range k.Clusters {
		appPods := k.GetAppPods(cluster)
		if len(appPods[app]) == 0 {
			return "", errors.Errorf("missing pod names for app %q", app)
		}

		pod := appPods[app][0]

		r, e := util.PodExec(k.Namespace, pod, "app", fmt.Sprintf("client --url %s", routesURL), true, k.Clusters[cluster])
		if e != nil {
			return "", errors.WithMessage(err, "failed to get routes")
		}
		routes += fmt.Sprintf("Routes From %s Cluster: \n", cluster)
		routes += r
	}

	return routes, nil
}

// getAppPods returns a copy of the appPods map. Should only be called by GetAppPods.
func (k *KubeInfo) getAppPods(cluster string) map[string][]string {
	k.appPods[cluster].PodsMutex.Lock()
	defer k.appPods[cluster].PodsMutex.Unlock()

	return k.deepCopy(k.appPods[cluster].Pods)
}

// setAppPods sets the app pods with a copy of the given map. Should only be called by GetAppPods.
func (k *KubeInfo) setAppPods(cluster string, newMap map[string][]string) {
	k.appPods[cluster].PodsMutex.Lock()
	defer k.appPods[cluster].PodsMutex.Unlock()

	k.appPods[cluster].Pods = k.deepCopy(newMap)
}

func (k *KubeInfo) deepCopy(src map[string][]string) map[string][]string {
	newMap := make(map[string][]string, len(src))
	for k, v := range src {
		newMap[k] = v
	}
	return newMap
}

func (k *KubeInfo) deployIstio() error {
	istioYaml := nonAuthInstallFileNamespace
	if *multiClusterDir != "" {
		if *splitHorizon {
			if !*authEnable {
				return errors.New("the split horizon test can only work with auth enabled")
			}
			istioYaml = mcSplitHorizonInstallFile
		} else {
			if *authEnable {
				istioYaml = mcAuthInstallFileNamespace
			} else {
				istioYaml = mcNonAuthInstallFileNamespace
			}
		}
	} else if *clusterWide {
		istioYaml = getClusterWideInstallFile()
	} else {
		if *authEnable {
			istioYaml = authInstallFileNamespace
		}
		if *trustDomainEnable {
			istioYaml = trustDomainFileNamespace
		}
		if *useGalleyConfigValidator {
			return errors.New("cannot enable useGalleyConfigValidator in one namespace tests")
		}
	}

	// Deploy the CNI if enabled
	if *useCNI {
		err := k.deployCNI()
		if err != nil {
			log.Errorf("Unable to deply Istio CNI")
			return err
		}

		timeout := time.Now().Add(maxCNIDeployTime)
		var CNIPodName string
		for time.Now().Before(timeout) {
			// Check if the CNI pod deployed
			if CNIPodName, err = util.GetPodName(k.Namespace, "k8s-app=istio-cni-node", k.KubeConfig); err == nil {
				break
			}

		}

		if CNIPodName == "" {
			return errors.New("timeout waiting for CNI to deploy")
		}

		// Check if the CNI Pod is running.  Note at this point only the CNI is deployed
		// and it will be the only pod in the namespace
		if CNIRunning := util.CheckPodsRunning(k.Namespace, k.KubeConfig); !CNIRunning {
			return errors.New("timeout waiting for CNI to become ready")
		}

	}

	var testIstioYaml string
	// Use the operator manifest when operator mode enabled
	if *useOperator {
		istioYaml = authOperatorInstallFile
		yamlDir := filepath.Join(istioInstallDir, istioYaml)
		baseIstioYaml := filepath.Join(k.ReleaseDir, yamlDir)
		testIstioYaml = filepath.Join(k.TmpDir, "yaml", istioYaml)
		util.CopyFile(baseIstioYaml, testIstioYaml)
		if err := util.KubeApply("istio-operator", testIstioYaml, k.KubeConfig); err != nil {
			log.Errorf("Istio operator %s deployment failed", testIstioYaml)
			return err
		}
		if err := k.waitForIstioOperator(); err != nil {
			log.Errorf("istio operator fails to deploy Istio: %v", err)
			return err
		}
	} else {
		// Create istio-system namespace
		if err := util.CreateNamespace(k.Namespace, k.KubeConfig); err != nil {
			log.Errorf("Unable to create namespace %s: %s", k.Namespace, err.Error())
			return err
		}

		// Apply istio-init and generate an Istio manifest for later
		yamlDir := filepath.Join(istioInstallDir, initInstallFile)
		baseIstioYaml := filepath.Join(k.ReleaseDir, yamlDir)
		testIstioYaml = filepath.Join(k.TmpDir, "yaml", istioYaml)
		if err := k.generateIstio(baseIstioYaml, testIstioYaml); err != nil {
			log.Errorf("Generating istio-init.yaml")
			return err
		}

		if err := util.KubeApply(k.Namespace, testIstioYaml, k.KubeConfig); err != nil {
			log.Errorf("istio-init.yaml  %s deployment failed", testIstioYaml)
			return err
		}

		// TODO(sdake): need a better synchronization
		time.Sleep(20 * time.Second)

		// Generate main manifest for application in the rest of this function
		// depending on E2E flags
		yamlDir = filepath.Join(istioInstallDir, istioYaml)
		baseIstioYaml = filepath.Join(k.ReleaseDir, yamlDir)
		testIstioYaml = filepath.Join(k.TmpDir, "yaml", istioYaml)

		if err := k.generateIstio(baseIstioYaml, testIstioYaml); err != nil {
			log.Errorf("Generating yaml %s failed", testIstioYaml)
			return err
		}

		if *multiClusterDir != "" {
			if err := k.createCacerts(false); err != nil {
				log.Infof("Failed to create Cacerts with namespace %s in primary cluster", k.Namespace)
			}
		}
		if err := util.KubeApply(k.Namespace, testIstioYaml, k.KubeConfig); err != nil {
			log.Errorf("Istio core %s deployment failed", testIstioYaml)
			return err
		}
	}

	if *multiClusterDir != "" {
		// Create namespace on any remote clusters
		if err := util.CreateNamespace(k.Namespace, k.RemoteKubeConfig); err != nil {
			log.Errorf("Unable to create namespace %s on remote cluster: %s", k.Namespace, err.Error())
			return err
		}

		if err := k.createCacerts(true); err != nil {
			log.Infof("Failed to create Cacerts with namespace %s in remote cluster", k.Namespace)
		}

		testIstioYaml := filepath.Join(k.TmpDir, "yaml", mcRemoteInstallFile)
		var err error
		if *splitHorizon {
			if *useAutomaticInjection {
				return errors.New("the split horizon test does not work with automatic injection")
			}

			err = k.generateRemoteIstioForSplitHorizon(testIstioYaml, remoteNetworkName, *proxyHub, *proxyTag)
		} else {
			err = k.generateRemoteIstio(testIstioYaml, *useAutomaticInjection, *proxyHub, *proxyTag)
		}
		if err != nil {
			log.Errorf("Generating Remote yaml %s failed", testIstioYaml)
			return err
		}
		if err := util.KubeApply(k.Namespace, testIstioYaml, k.RemoteKubeConfig); err != nil {
			log.Errorf("Remote Istio %s deployment failed", testIstioYaml)
			return err
		}
		if err := util.CreateMultiClusterSecret(k.Namespace, k.RemoteKubeConfig, k.KubeConfig); err != nil {
			log.Errorf("Unable to create remote cluster secret on local cluster %s", err.Error())
			return err
		}
		time.Sleep(10 * time.Second)
	}

	if *useAutomaticInjection {
		baseSidecarInjectorYAML := util.GetResourcePath(filepath.Join(istioInstallDir, *sidecarInjectorFile))
		testSidecarInjectorYAML := filepath.Join(k.TmpDir, "yaml", *sidecarInjectorFile)
		if err := k.generateSidecarInjector(baseSidecarInjectorYAML, testSidecarInjectorYAML); err != nil {
			log.Errorf("Generating sidecar injector yaml failed")
			return err
		}
		if err := util.KubeApply(k.Namespace, testSidecarInjectorYAML, k.KubeConfig); err != nil {
			log.Errorf("Istio sidecar injector %s deployment failed", testSidecarInjectorYAML)
			return err
		}
	}

	if err := util.CheckDeployments(k.Namespace, maxDeploymentRolloutTime, k.KubeConfig); err != nil {
		return err
	}

	if *useGalleyConfigValidator {
		timeout := time.Now().Add(maxValidationReadyCheckTime)
		var validationReady bool
		for time.Now().Before(timeout) {
			if _, err := util.ShellSilent("kubectl get validatingwebhookconfiguration istio-galley --kubeconfig=%s", k.KubeConfig); err == nil {
				validationReady = true
				break
			}
		}
		if !validationReady {
			return errors.New("timeout waiting for validatingwebhookconfiguration istio-galley to be created")
		}

		if err := k.waitForValdiationWebhook(); err != nil {
			return err
		}
	}
	return nil
}

// DeployTiller deploys tiller in Istio mesh or returns error
func (k *KubeInfo) DeployTiller() error {
	// no need to deploy tiller when Istio is deployed using helm as Tiller is already deployed as part of it.
	if *installer == helmInstallerName {
		return nil
	}

	yamlDir := filepath.Join(istioInstallDir+"/"+helmInstallerName, helmServiceAccountFile)
	baseHelmServiceAccountYaml := filepath.Join(k.ReleaseDir, yamlDir)
	return k.deployTiller(baseHelmServiceAccountYaml)
}

func (k *KubeInfo) deployTiller(yamlFileName string) error {
	// apply helm service account
	if err := util.KubeApply("kube-system", yamlFileName, k.KubeConfig); err != nil {
		log.Errorf("Failed to apply %s", yamlFileName)
		return err
	}

	// deploy tiller, helm cli is already available
	if err := util.HelmInit("tiller"); err != nil {
		log.Errorf("Failed to init helm tiller")
		return err
	}

	// Wait until Helm's Tiller is running
	return util.HelmTillerRunning()
}

var (
	dummyValidationRule = `
apiVersion: "config.istio.io/v1alpha2"
kind: rule
metadata:
  name: validation-readiness-dummy-rule
spec:
  match: request.headers["foo"] == "bar"
  actions:
  - handler: validation-readiness-dummy
    instances:
    - validation-readiness-dummy
`
)

// Wait for Kubernetes to become active within kubernetesReadinessTimeout period
// This operation only retreives the pods in the kube-system namespace
// (TODO) sdake: This may be insufficient for a complete readiness check of Kubernetes
func (k *KubeInfo) waitForKubernetes() error {
	log.Info("Waiting for Kubernetes to become responsive")
	return retry.UntilSuccess(func() error {
		_, err := k.KubeAccessor.GetPods("kube-system")
		return err
	}, retry.Delay(kubernetesReadinessInterval), retry.Timeout(kubernetesReadinessTimeout))
}

func (k *KubeInfo) waitForValdiationWebhook() error {

	add := fmt.Sprintf(`cat << EOF | kubectl --kubeconfig=%s apply -f -
%s
EOF`, k.KubeConfig, dummyValidationRule)

	remove := fmt.Sprintf(`cat << EOF | kubectl --kubeconfig=%s delete -f -
%s
EOF`, k.KubeConfig, dummyValidationRule)

	log.Info("Creating dummy rule to check for validation webhook readiness")
	timeout := time.Now().Add(validationWebhookReadinessTimeout)
	for {
		if time.Now().After(timeout) {
			return errors.New("timeout waiting for validation webhook readiness")
		}

		out, err := util.ShellSilent(add)
		if err == nil && !strings.Contains(out, "connection refused") {
			break
		}

		log.Errorf("Validation webhook not ready yet: %v %v", out, err)
		time.Sleep(validationWebhookReadinessFreq)

	}
	util.ShellSilent(remove) // nolint: errcheck
	log.Info("Validation webhook is ready")
	return nil
}

func (k *KubeInfo) waitForIstioOperator() error {

	get := fmt.Sprintf(`kubectl --kubeconfig=%s get iop example-istiocontrolplane -n istio-operator -o yaml`, k.KubeConfig)
	timeout := time.Now().Add(istioOperatorTimeout)
	for {
		if time.Now().After(timeout) {
			return errors.New("timeout waiting for istio operator to deploy Istio")
		}
		out, err := util.ShellSilent(get)
		if err == nil && strings.Contains(out, "HEALTHY") {
			break
		}

		log.Warnf("istio-operator is still deploying Istio: %v", err)
		time.Sleep(istioOperatorFreq)

	}
	log.Info("istio operator succeeds to deploy Istio")
	return nil
}

func (k *KubeInfo) deployCRDs(kubernetesCRD string) error {
	yamlFileName := filepath.Join(istioInstallDir, helmInstallerName, "istio-init", "files", kubernetesCRD)
	yamlFileName = filepath.Join(k.ReleaseDir, yamlFileName)

	// deploy CRDs first
	if err := util.KubeApply("kube-system", yamlFileName, k.KubeConfig); err != nil {
		return err
	}
	return nil
}

func (k *KubeInfo) deployIstioWithHelm() error {
	// Note: When adding a CRD to the install, a new CRDFile* constant is needed
	// This slice contains the list of CRD files installed during testing
	istioCRDFileNames := []string{allCRDInstallFile, mixerCRDInstallFile}
	// deploy all CRDs in Istio first
	for _, yamlFileName := range istioCRDFileNames {
		if err := k.deployCRDs(yamlFileName); err != nil {
			return fmt.Errorf("failed to apply all Istio CRDs from file: %s with error: %v", yamlFileName, err)
		}
	}

	// install istio helm chart, which includes addon
	isSecurityOn := false
	if *authEnable {
		// enable mTLS
		isSecurityOn = true
	}

	// construct setValue to pass into helm install
	// mTLS
	setValue := "--set global.mtls.enabled=" + strconv.FormatBool(isSecurityOn)
	// side car injector
	if *useAutomaticInjection {
		setValue += " --set sidecarInjectorWebhook.enabled=true"
	}

	if *useMCP && *useGalleyConfigValidator {
		setValue += " --set galley.enabled=true --set global.useMCP=true --set global.configValidation=true"
	} else if *useMCP {
		setValue += " --set galley.enabled=true --set global.useMCP=true --set global.configValidation=false"
	} else if *useGalleyConfigValidator {
		setValue += " --set galley.enabled=true --set global.useMCP=false --set global.configValidation=true"
	} else {
		setValue += " --set galley.enabled=false --set global.useMCP=false"
	}

	if *outboundTrafficPolicy != "" {
		setValue += " --set global.outboundTrafficPolicy.mode=" + *outboundTrafficPolicy
	}

	if *enableEgressGateway {
		setValue += " --set gateways.istio-egressgateway.enabled=true"
	}

	// hubs and tags replacement.
	// Helm chart assumes hub and tag are the same among multiple istio components.
	if *pilotHub != "" && *pilotTag != "" {
		setValue += " --set-string global.hub=" + *pilotHub + " --set-string global.tag=" + *pilotTag
	}

	if !*clusterWide {
		setValue += " --set global.oneNamespace=true"
	}

	// enable helm test for istio
	setValue += " --set global.enableHelmTest=true"

	// add additional values passed from test
	for _, v := range helmSetValues {
		setValue += " --set " + v
	}
	err := util.HelmClientInit()
	if err != nil {
		// helm client init
		log.Errorf("Helm client init error: %v", err)
		return err
	}
	// Generate dependencies for Helm
	workDir := filepath.Join(k.ReleaseDir, istioHelmInstallDir)
	valFile := ""
	if *valueFile != "" {
		valFile = filepath.Join(workDir, *valueFile)
	}

	if *useCNI {
		if err = k.deployCNI(); err != nil {
			log.Error("Failed to deploy the Istio CNI")
			return err
		}
		setValue += " --set istio_cni.enabled=true"
	}

	// helm install dry run - dry run seems to have problems
	// with CRDs even in 2.9.2, pre-install is not executed
	err = util.HelmInstallDryRun(workDir, istioHelmChartName, valFile, k.Namespace, setValue)
	if err != nil {
		// dry run fail, let's fail early
		log.Errorf("Helm dry run of istio chart failed %s, valueFile=%s, setValue=%s, namespace=%s",
			istioHelmInstallDir, valFile, setValue, k.Namespace)
		return err
	}

	// helm install
	err = util.HelmInstall(workDir, istioHelmChartName, valFile, k.Namespace, setValue)
	if err != nil {
		log.Errorf("Helm install istio chart failed %s, valueFile=%s, setValue=%s, namespace=%s",
			istioHelmInstallDir, valFile, setValue, k.Namespace)
		return err
	}

	if *useGalleyConfigValidator {
		if err := k.waitForValdiationWebhook(); err != nil {
			return err
		}
	}

	return nil
}

func (k *KubeInfo) executeHelmTest() error {
	if !util.CheckPodsRunning(k.Namespace, k.KubeConfig) {
		return fmt.Errorf("can't get all pods running")
	}

	if err := util.HelmTest("istio"); err != nil {
		return fmt.Errorf("helm test istio failed")
	}

	return nil
}

func updateInjectImage(name, module, hub, tag string, content []byte) []byte {
	image := []byte(fmt.Sprintf("%s: %s/%s:%s", name, hub, module, tag))
	r := regexp.MustCompile(fmt.Sprintf("%s: .*(\\/%s):.*", name, module))
	return r.ReplaceAllLiteral(content, image)
}

func updateInjectVersion(version string, content []byte) []byte {
	versionLine := []byte(fmt.Sprintf("version: %s", version))
	r := regexp.MustCompile("version: .*")
	return r.ReplaceAllLiteral(content, versionLine)
}

func (k *KubeInfo) generateSidecarInjector(src, dst string) error {
	content, err := ioutil.ReadFile(src)
	if err != nil {
		log.Errorf("Cannot read original yaml file %s", src)
		return err
	}

	if !*clusterWide {
		content = replacePattern(content, istioSystem, k.Namespace)
	}

	if *pilotHub != "" && *pilotTag != "" {
		content = updateImage("sidecar_injector", *pilotHub, *pilotTag, content)
		content = updateInjectVersion(*pilotTag, content)
		content = updateInjectImage("proxyImage", "proxy", *proxyHub, *proxyTag, content)
	}

	err = ioutil.WriteFile(dst, content, 0600)
	if err != nil {
		log.Errorf("Cannot write into generate sidecar injector file %s", dst)
	}
	return err
}

func replacePattern(content []byte, src, dest string) []byte {
	r := []byte(dest)
	p := regexp.MustCompile(src)
	content = p.ReplaceAllLiteral(content, r)
	return content
}

// This code is in need of a reimplementation and some rethinking. An in-place
// modification on the raw manifest is the wrong approach. This model is also
// not portable to our future around IstioOperator where we don't necessarily
// have a manifest to in place modify...
func (k *KubeInfo) generateIstio(src, dst string) error {
	content, err := ioutil.ReadFile(src)
	if err != nil {
		log.Errorf("Cannot read original yaml file %s", src)
		return err
	}

	if !*clusterWide {
		content = replacePattern(content, istioSystem, k.Namespace)
		// Customize mixer's configStoreURL to limit watching resources in the testing namespace.
		vs := url.Values{}
		vs.Add("ns", *namespace)
		content = replacePattern(content, "--configStoreURL=k8s://", "--configStoreURL=k8s://?"+vs.Encode())
	}

	// Replace long refresh delays with short ones for the sake of tests.
	// note these config nobs aren't exposed to helm
	content = replacePattern(content, "connectTimeout: 10s", "connectTimeout: 1s")
	content = replacePattern(content, "drainDuration: 45s", "drainDuration: 2s")
	content = replacePattern(content, "parentShutdownDuration: 1m0s", "parentShutdownDuration: 3s")

	// A very flimsy and unreliable regexp to replace delays in ingress gateway pod Spec
	// note these config nobs aren't exposed to helm
	content = replacePattern(content, "'10s' #connectTimeout", "'1s' #connectTimeout")
	content = replacePattern(content, "'45s' #drainDuration", "'2s' #drainDuration")
	content = replacePattern(content, "'1m0s' #parentShutdownDuration", "'3s' #parentShutdownDuration")

	if k.BaseVersion == "" {
		if *mixerHub != "" && *mixerTag != "" {
			content = updateImage("mixer", *mixerHub, *mixerTag, content)
		}
		if *pilotHub != "" && *pilotTag != "" {
			content = updateImage("pilot", *pilotHub, *pilotTag, content)
		}
		if *proxyHub != "" && *proxyTag != "" {
			//Need to be updated when the string "proxy" is changed as the default image name
			content = updateImage("proxyv2", *proxyHub, *proxyTag, content)
		}
		if *sidecarInjectorHub != "" && *sidecarInjectorTag != "" {
			//Need to be updated when the string "proxy" is changed as the default image name
			content = updateImage("sidecar_injector", *sidecarInjectorHub, *sidecarInjectorTag, content)
		}
		if *caHub != "" && *caTag != "" {
			//Need to be updated when the string "citadel" is changed
			content = updateImage("citadel", *caHub, *caTag, content)
		}
		if *galleyHub != "" && *galleyTag != "" {
			//Need to be updated when the string "citadel" is changed
			content = updateImage("galley", *galleyHub, *galleyTag, content)
		}
		if *imagePullPolicy != "" {
			content = updateImagePullPolicy(*imagePullPolicy, content)
		}
	}

	if *localCluster {
		content = []byte(strings.Replace(string(content), util.LoadBalancerServiceType,
			util.NodePortServiceType, 1))
	}

	if *splitHorizon {
		registryName := filepath.Base(k.RemoteKubeConfig)
		content = replacePattern(content, "N2_REGISTRY_TOKEN", registryName)
	}

	err = ioutil.WriteFile(dst, content, 0600)
	if err != nil {
		log.Errorf("Cannot write into generated yaml file %s", dst)
	}
	return err
}

func updateImage(module, hub, tag string, content []byte) []byte {
	image := []byte(fmt.Sprintf("image: %s/%s:%s", hub, module, tag))
	r := regexp.MustCompile(fmt.Sprintf("image: .*(\\/%s):.*", module))
	return r.ReplaceAllLiteral(content, image)
}

func updateImagePullPolicy(policy string, content []byte) []byte {
	image := []byte(fmt.Sprintf("imagePullPolicy: %s", policy))
	r := regexp.MustCompile("imagePullPolicy:.*")
	return r.ReplaceAllLiteral(content, image)
}

// deployCNI will deploy the Istio CNI components on to the cluster.
// There are two prerequisites. First, the helm repo must be added in the
// local environemt.  The tests won't add the repo for any code paths.
// The Istio helm value istio_cni.enabled needs to be set when the test
// is being run using the generated yamls (vs. helm install).  The ENV
// variable ENABLE_ISTIO_CNI can be used.
func (k *KubeInfo) deployCNI() error {
	// The CNI is not compatible with all test options.  TODO - enumerate others
	if *multiClusterDir != "" {
		log.Errorf("CNI is not enabled for the multicluster test")
	}
	log.Info("Deploy Istio CNI components")
	// Some environments will require additional options to be set or changed
	// (e.g. GKE environments need the bin directory to be changed from the default
	setValue := " --set-string hub=" + *cniHub + " --set-string tag=" + *cniTag
	setValue += " --set excludeNamespaces={} --set pullPolicy=IfNotPresent --set logLevel=debug"
	if extraHelmValues := os.Getenv("EXTRA_HELM_SETTINGS"); extraHelmValues != "" {
		setValue += extraHelmValues
	}
	if *installer == helmInstallerName {
		//Assumes the CNI helm repo is available alongside the istio repo
		err := util.HelmInstall(*cniHelmRepo, "istio-cni", "", k.Namespace, setValue)
		if err != nil {
			log.Errorf("Helm install istio-cni chart failed, setValue=%s, namespace=%s",
				setValue, k.Namespace)
			return err
		}
		return err
	}
	chartDir := filepath.Join(k.TmpDir, "cniChartDir")
	err := util.HelmFetch(*cniHelmRepo, chartDir)
	if err != nil {
		log.Errorf("Helm fetch of %s failed", *cniHelmRepo)
		return err
	}
	outputFile := filepath.Join(k.TmpDir, "istio-cni_install.yaml")
	chartDir = filepath.Join(chartDir, "istio-cni")
	err = util.HelmTemplate(chartDir, "istio-cni", k.Namespace, setValue, outputFile)
	if err != nil {
		log.Errorf("Helm template of istio-cni failed")
		return err
	}
	err = util.KubeApply(k.Namespace, outputFile, k.KubeConfig)
	if err != nil {
		log.Errorf("Kubeapply istio-cni failed")
	}
	return err
}