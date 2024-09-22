package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"gopkg.in/yaml.v2"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var protectedNamespaces []string
var protectedNamespacesMutex sync.RWMutex

type Config struct {
	ProtectedNamespaces []string `yaml:"protectedNamespaces"`
}

func main() {
	// Set up in-cluster Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Failed to create in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create Kubernetes client: %v", err)
	}
	// Ensure the 'resources-shield' namespace exists
	if err := ensureNamespace(clientset, "resources-shield"); err != nil {
		klog.Fatalf("Failed to ensure namespace: %v", err)
	}

	// Namespace and name of the ConfigMap
	namespace := "resources-shield"
	configMapName := "resources-shield-cm"

	// Load initial configuration
	if err := loadConfig(clientset, namespace, configMapName); err != nil {
		klog.Fatalf("Failed to load initial config: %v", err)
	}

	// Always include 'resources-shield' operator in protected namespaces
	protectedNamespaces = append(protectedNamespaces, "resources-shield")

	// Start watching the ConfigMap in a separate goroutine
	go watchConfigMap(clientset, namespace, configMapName)

	// Start the HTTP server
	http.HandleFunc("/validate", handleValidation)
	server := &http.Server{
		Addr: ":8443",
	}

	certFile := "/tls/tls.crt"
	keyFile := "/tls/tls.key"

	klog.Info("Starting webhook server...")
	if err := server.ListenAndServeTLS(certFile, keyFile); err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
}

func ensureNamespace(clientset *kubernetes.Clientset, namespace string) error {
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err == nil {
		// Namespace exists
		return nil
	}
	if errors.IsNotFound(err) {
		// Namespace does not exist, create it
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create namespace %s: %v", namespace, err)
		}
		return nil
	}
	return fmt.Errorf("failed to get namespace %s: %v", namespace, err)
}

func loadConfig(clientset *kubernetes.Clientset, namespace, configMapName string) error {
	configMap, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Failed to get ConfigMap: %v", err)
	}

	data, ok := configMap.Data["protected-resources-shield-list.yaml"]
	if !ok {
		return fmt.Errorf("Key 'protected-resources-shield-list.yaml' not found in ConfigMap")
	}

	var config Config
	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		return fmt.Errorf("Failed to parse config data: %v", err)
	}

	protectedNamespacesMutex.Lock()
	protectedNamespaces = config.ProtectedNamespaces
	protectedNamespacesMutex.Unlock()

	klog.Infof("Protected namespaces initialized: %v", protectedNamespaces)
	return nil
}

func watchConfigMap(clientset *kubernetes.Clientset, namespace, configMapName string) {
	for {
		watcher, err := clientset.CoreV1().ConfigMaps(namespace).Watch(context.TODO(), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", configMapName),
		})
		if err != nil {
			klog.Errorf("Failed to watch ConfigMap: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for event := range watcher.ResultChan() {
			switch event.Type {
			case watch.Added, watch.Modified:
				configMap, ok := event.Object.(*corev1.ConfigMap)
				if !ok {
					klog.Errorf("Unexpected type: %T", event.Object)
					continue
				}

				data, ok := configMap.Data["protected-resources-shield-list.yaml"]
				if !ok {
					klog.Errorf("Key 'protected-resources-shield-list.yaml' not found in ConfigMap")
					continue
				}

				var config Config
				if err := yaml.Unmarshal([]byte(data), &config); err != nil {
					klog.Errorf("Failed to parse config data: %v", err)
					continue
				}

				protectedNamespacesMutex.Lock()
				protectedNamespaces = config.ProtectedNamespaces
				protectedNamespacesMutex.Unlock()

				klog.Infof("Protected namespaces updated: %v", protectedNamespaces)
			case watch.Deleted:
				klog.Warning("ConfigMap deleted, clearing protected namespaces")
				protectedNamespacesMutex.Lock()
				protectedNamespaces = nil
				protectedNamespacesMutex.Unlock()
			case watch.Error:
				klog.Errorf("Error watching ConfigMap: %v", event.Object)
			}
		}
		klog.Info("Watcher closed, restarting")
		time.Sleep(5 * time.Second)
	}
}

func handleValidation(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Could not read request body: %v", err)
		http.Error(w, "Could not read request body", http.StatusBadRequest)
		return
	}

	review := admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, &review); err != nil {
		klog.Errorf("Could not unmarshal request: %v", err)
		http.Error(w, "Could not unmarshal request", http.StatusBadRequest)
		return
	}

	response := admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	// Check if it's a DELETE operation on a Namespace
	if review.Request.Kind.Kind == "Namespace" && review.Request.Operation == admissionv1.Delete {
		namespaceName := review.Request.Name

		// Check if the namespace is in the protected list
		for _, protected := range protectedNamespaces {
			if namespaceName == protected {
				response.Allowed = false
				response.Result = &metav1.Status{
					Status:  "Failure",
					Message: "Deletion of this namespace is not allowed by resources-shield policy.",
					Reason:  metav1.StatusReasonForbidden,
					Code:    http.StatusForbidden,
				}
				break
			}
		}
	}

	// Deny deletion of operator's own resources
	if review.Request.Operation == admissionv1.Delete {
		resourceKind := review.Request.Kind.Kind
		resourceName := review.Request.Name
		resourceNamespace := review.Request.Namespace

		// Protected cluster-scoped resources
		protectedClusterResources := map[string][]string{
			"Namespace":          {"resources-shield"},
			"ClusterRole":        {"resources-shield-crole"},
			"ClusterRoleBinding": {"resources-shield-crbind"},
		}

		// Protected namespaced resources in 'resources-shield' namespace
		protectedNamespacedResources := map[string][]string{
			"Deployment":                     {"resources-shield-deploy"},
			"Service":                        {"resources-shield-svc"},
			"ValidatingWebhookConfiguration": {"resources-shield-validating-webhook"},
			"ServiceAccount":                 {"resources-shield-sa"},
			"ConfigMap":                      {"resources-shield-cm"},
			"secret":                         {"resources-shield-certs"},
		}

		// Check cluster-scoped resources
		if names, exists := protectedClusterResources[resourceKind]; exists {
			for _, name := range names {
				if resourceName == name {
					// Deny deletion
					response.Allowed = false
					response.Result = &metav1.Status{
						Status:  "Failure",
						Message: fmt.Sprintf("Deletion of %s '%s' is not allowed by resources-shield policy.", resourceKind, resourceName),
						Reason:  metav1.StatusReasonForbidden,
						Code:    http.StatusForbidden,
					}
					break
				}
			}
		}

		// Check namespaced resources
		if names, exists := protectedNamespacedResources[resourceKind]; exists {
			if resourceNamespace == "resources-shield" {
				for _, name := range names {
					if resourceName == name {
						// Deny deletion
						response.Allowed = false
						response.Result = &metav1.Status{
							Status:  "Failure",
							Message: fmt.Sprintf("Deletion of %s '%s' in namespace '%s' is not allowed by resources-shield policy.", resourceKind, resourceName, resourceNamespace),
							Reason:  metav1.StatusReasonForbidden,
							Code:    http.StatusForbidden,
						}
						break
					}
				}
			}
		}
	}

	// Prepare the response
	respReview := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: &response,
	}

	respBytes, err := json.Marshal(respReview)
	if err != nil {
		klog.Errorf("Could not marshal response: %v", err)
		http.Error(w, "Could not marshal response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		klog.Errorf("Could not write response: %v", err)
	}
}
