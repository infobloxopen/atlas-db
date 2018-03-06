package mysql

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	atlas "github.com/infobloxopen/atlas-db/pkg/apis/db/v1alpha1"
	"github.com/infobloxopen/atlas-db/pkg/server/plugin"
)

const (
	defaultImage   = "mysql"
	defaultVersion = "5.6"
	containerName  = "mysqld"
	portName       = "mysql"
	defaultPort    = 3306
	configPath     = "/etc/mysql/conf.d"
	dataPath       = "/var/lib/mysql"
	superUserPwEnv = "MYSQL_ROOT_PASSWORD"
)

type MySQLPlugin atlas.MySQLPlugin

func Convert(a *atlas.MySQLPlugin) *MySQLPlugin {
	p := MySQLPlugin(*a)
	return &p
}

func (p *MySQLPlugin) Name() string {
	return "MySQL"
}

// CreatePod creates a new Pod for a DatabaseServer resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DatabaseServer resource that 'owns' it.
func (p *MySQLPlugin) CreatePod(key string, s *atlas.DatabaseServer) *corev1.Pod {
	labels := map[string]string{
		"controller": s.Name,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(s, schema.GroupVersionKind{
					Group:   atlas.SchemeGroupVersion.Group,
					Version: atlas.SchemeGroupVersion.Version,
					Kind:    "DatabaseServer",
				}),
			},
		},
		Spec: corev1.PodSpec{
			Volumes: s.Spec.Volumes,
			Containers: []corev1.Container{
				{
					Name:  containerName,
					Image: plugin.PodImage(p.Image, p.Version, defaultImage, defaultVersion),
					Env: []corev1.EnvVar{
						{
							Name:      superUserPwEnv,
							Value:     s.Spec.SuperUserPassword,
							ValueFrom: s.Spec.SuperUserPasswordFrom,
						},
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          portName,
							Protocol:      "TCP",
							ContainerPort: plugin.PodContainerPort(s.Spec.Port, defaultPort),
						},
					},
				},
			},
		},
	}
	c := &pod.Spec.Containers[0]
	if p.ConfigVolume != "" {
		plugin.AddMount(c, p.ConfigVolume, true, configPath)
	}

	if p.DataVolume != "" {
		plugin.AddMount(c, p.DataVolume, false, dataPath)
	}

	return pod
}

// DiffPod returns a string describing differences between the spec and the pod,
// or the empty string if there are no differences
func (p *MySQLPlugin) DiffPod(key string, s *atlas.DatabaseServer, pod *corev1.Pod) string {
	diffs := []string{}
	var c *corev1.Container
	if len(pod.Spec.Containers) != 1 {
		diffs = append(diffs, "incorrect number of containers")
	} else {
		c = &pod.Spec.Containers[0]
	}

	expImg := plugin.PodImage(p.Image, p.Version, defaultImage, defaultVersion)
	if c != nil && c.Image != expImg {
		diffs = append(diffs, "incorrect image")
	}

	//TODO: Check volumes, etc.
	return strings.Join(diffs, "; ")
}
