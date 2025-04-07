package tas

import (
	"context"

	trustyaiopendatahubiov1alpha1 "github.com/trustyai-explainability/trustyai-service-operator/api/tas/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	tlsMountPath    = "/etc/trustyai/tls"
	odhCARootPath   = "/etc/ssl/certs/ca-bundle.crt"
	odhCABundleName = "odh-trusted-ca-bundle"
)

// TLSCertVolumes holds the volume and volume mount for the TLS certificates
type TLSCertVolumes struct {
	volume      corev1.Volume
	volumeMount corev1.VolumeMount
}

// createFor creates the required volumes and volume mount for the TLS certificates for a specific Kubernetes secret
func (cert *TLSCertVolumes) createFor(instance *trustyaiopendatahubiov1alpha1.TrustyAIService) {
	volume := corev1.Volume{
		Name: instance.Name + "-internal",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: instance.Name + "-internal",
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      instance.Name + "-internal",
		MountPath: tlsMountPath,
		ReadOnly:  true,
	}
	cert.volume = volume
	cert.volumeMount = volumeMount
}

type CustomCertificatesBundle struct {
	IsDefined     bool
	VolumeName    string
	ConfigMapName string
	MountPath     string
	SubPath       string
	CertData      string
}

func (r *TrustyAIServiceReconciler) GetCustomCertificatesBundle(ctx context.Context, instance *trustyaiopendatahubiov1alpha1.TrustyAIService) CustomCertificatesBundle {
	var customCertificatesBundle CustomCertificatesBundle

	// Check for odh-trusted-ca-bundle ConfigMap
	odhCABundle := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: odhCABundleName, Namespace: instance.Namespace}, odhCABundle)
	if err == nil {
		// Has data?
		if odhCABundle.Data["ca-bundle.crt"] != "" {
			log.FromContext(ctx).Info("Found odh-trusted-ca-bundle ConfigMap with ca-bundle.crt. Using it for CA certificates.")
			customCertificatesBundle.IsDefined = true
			customCertificatesBundle.VolumeName = odhCABundleName
			customCertificatesBundle.ConfigMapName = odhCABundleName
			customCertificatesBundle.MountPath = odhCARootPath
			customCertificatesBundle.SubPath = "ca-bundle.crt"
			customCertificatesBundle.CertData = odhCABundle.Data["ca-bundle.crt"]
			return customCertificatesBundle
		} else if odhCABundle.Data["odh-ca-bundle.crt"] != "" {
			log.FromContext(ctx).Info("Found odh-trusted-ca-bundle ConfigMap with odh-ca-bundle.crt. Using it for CA certificates.")
			customCertificatesBundle.IsDefined = true
			customCertificatesBundle.VolumeName = odhCABundleName
			customCertificatesBundle.ConfigMapName = odhCABundleName
			customCertificatesBundle.MountPath = odhCARootPath
			customCertificatesBundle.SubPath = "odh-ca-bundle.crt"
			customCertificatesBundle.CertData = odhCABundle.Data["odh-ca-bundle.crt"]
			return customCertificatesBundle
		}
	}

	tlsCertSecret := &corev1.Secret{}
	tlsErr := r.Get(ctx, types.NamespacedName{Name: instance.Name + "-tls", Namespace: instance.Namespace}, tlsCertSecret)
	if tlsErr == nil && tlsCertSecret.Data["tls.crt"] != nil {
		log.FromContext(ctx).Info("Using service TLS certificate as CA certificate")
		customCertificatesBundle.IsDefined = true
		customCertificatesBundle.VolumeName = instance.Name + "-tls-ca"
		customCertificatesBundle.MountPath = odhCARootPath
		customCertificatesBundle.CertData = string(tlsCertSecret.Data["tls.crt"])
		return customCertificatesBundle
	}

	labelSelector := client.MatchingLabels{caBundleAnnotation: "true"}
	configMapNames, err := r.getConfigMapNamesWithLabel(ctx, instance.Namespace, labelSelector)
	caNotFoundMessage := "CA bundle ConfigMap named '" + caBundleName + "' not found. Not using custom CA bundle."
	if err != nil {
		log.FromContext(ctx).Info(caNotFoundMessage)
		customCertificatesBundle.IsDefined = false
	} else {
		found := false
		for _, configMapName := range configMapNames {
			if configMapName == caBundleName {
				found = true
				break
			}
		}
		if found {
			caBundle := &corev1.ConfigMap{}
			err := r.Get(ctx, types.NamespacedName{Name: caBundleName, Namespace: instance.Namespace}, caBundle)
			if err == nil && caBundle.Data["ca-bundle.crt"] != "" {
				log.FromContext(ctx).Info("Found trusted CA bundle ConfigMap. Using custom CA bundle.")
				customCertificatesBundle.IsDefined = true
				customCertificatesBundle.VolumeName = caBundleName
				customCertificatesBundle.ConfigMapName = caBundleName
				customCertificatesBundle.MountPath = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
				customCertificatesBundle.SubPath = "ca-bundle.crt"
				customCertificatesBundle.CertData = caBundle.Data["ca-bundle.crt"]
			} else {
				log.FromContext(ctx).Info("Found CA bundle ConfigMap but it has no ca-bundle.crt data")
				customCertificatesBundle.IsDefined = false
			}
		} else {
			log.FromContext(ctx).Info(caNotFoundMessage)
			customCertificatesBundle.IsDefined = false
		}
	}
	return customCertificatesBundle
}
