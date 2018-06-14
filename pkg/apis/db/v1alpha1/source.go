package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ValueSource struct {
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *corev1.ConfigMapKeySelector
	// Selects a key of a secret in the pod's namespace.
	// +optional
	SecretKeyRef *corev1.SecretKeySelector
}

func (v *ValueSource) ToEnvVarSource() *corev1.EnvVarSource {
	if v == nil {
		return nil
	}
	return &corev1.EnvVarSource{
		ConfigMapKeyRef: v.ConfigMapKeyRef,
		SecretKeyRef:    v.SecretKeyRef,
	}
}

func (v *ValueSource) Resolve(c kubernetes.Interface, ns string) (string, error) {
	if v == nil {
		return "", fmt.Errorf("received nil ValueSource in Resolve")
	}

	if v.ConfigMapKeyRef != nil {
		cm, err := c.CoreV1().ConfigMaps(ns).Get(v.ConfigMapKeyRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if s, ok := cm.Data[v.ConfigMapKeyRef.Key]; ok {
			return s, nil
		}
		return "", fmt.Errorf("key '%s' not found in config map '%s.%s'",
			v.ConfigMapKeyRef.Key, ns, v.ConfigMapKeyRef.Name)
	}

	if v.SecretKeyRef != nil {
		sec, err := c.CoreV1().Secrets(ns).Get(v.SecretKeyRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if s, ok := sec.Data[v.SecretKeyRef.Key]; ok {
			return string(s), nil
		}
		return "", fmt.Errorf("key '%s' not found in secrets '%s.%s'",
			v.SecretKeyRef.Key, ns, v.SecretKeyRef.Name)
	}

	return "", nil
}
