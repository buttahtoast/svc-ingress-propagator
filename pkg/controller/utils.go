package controller

const WellKnownIngressAnnotation = "kubernetes.io/ingress.class"
const MetaBase = "ingress-propagator.buttah.cloud"

var LabelManaged = MetaBase + "/managed-by"
var LabelPropagator = MetaBase + "/propagator"

const IssuerNamespacedAnnotation = "cert-manager.io/issuer"
const IssuerClusterAnnotation = "cert-manager.io/cluster-issuer"

func stringSliceContains(slice []string, element string) bool {
	for _, sliceElement := range slice {
		if sliceElement == element {
			return true
		}
	}
	return false
}
