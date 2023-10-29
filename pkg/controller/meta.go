package controller

const WellKnownIngressAnnotation = "kubernetes.io/ingress.class"
const MetaBase = "ingress-propagator.buttah.cloud"

var LabelManaged = MetaBase + "/managed-by"
var LabelPropagator = MetaBase + "/propagator"
