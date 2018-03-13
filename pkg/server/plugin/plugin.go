package plugin

import (
	"strings"
	corev1 "k8s.io/api/core/v1"

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
)

// Plugin should be implemented by all plugins
type Plugin interface {
	Name() string
	DatabasePlugin() DatabasePlugin
	Dsn(string, *atlas.DatabaseServer) string
}

// CloudPlugin is for instances created by an IaaS platform
type CloudPlugin interface {
	SyncCloud(string, *atlas.DatabaseServer) error
}

// PodPlugin is for instances backed by a Pod
// Intended primarily for dev, not prod
type PodPlugin interface {
	CreatePod(string, *atlas.DatabaseServer) *corev1.Pod
	DiffPod(string, *atlas.DatabaseServer, *corev1.Pod) string
}

type DatabasePlugin interface {
	SyncDatabase(*atlas.Database,string) error
}

func PodImage(image, version, defImage, defVersion string) string {
	if image == "" {
		image = defImage
	}
	if !strings.ContainsRune(image, ':') {
		if version == "" {
			version = defVersion
		}
		image = image + ":" + version
	}
	return image
}

func PodContainerPort(port, defaultPort int32) int32 {
	if port == 0 {
		return defaultPort
	}

	return port
}

func AddMount(c *corev1.Container, name string, ro bool, path string) {
	c.VolumeMounts = append(c.VolumeMounts,
		corev1.VolumeMount{
			Name:      name,
			ReadOnly:  ro,
			MountPath: path,
		})
}

func GetDsn(d *atlas.Database) {
}

