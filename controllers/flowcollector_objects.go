package controllers

import (
	_ "embed"

	"github.com/netobserv/network-observability-operator/controllers/constants"
	"github.com/netobserv/network-observability-operator/pkg/helper"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:embed infra_health_dashboard.json
var healthDashboardEmbed string

const (
	downstreamLabelKey       = "openshift.io/cluster-monitoring"
	downstreamLabelValue     = "true"
	roleSuffix               = "-metrics-reader"
	monitoringServiceAccount = "prometheus-k8s"
	monitoringNamespace      = "openshift-monitoring"
	dashboardCMNamespace     = "openshift-config-managed"
	dashboardCMAnnotation    = "console.openshift.io/dashboard"

	flowDashboardCMName = "grafana-dashboard-netobserv-flow-metrics"
	flowDashboardCMFile = "netobserv-flow-metrics.json"

	healthDashboardCMName = "grafana-dashboard-netobserv-health"
	healthDashboardCMFile = "netobserv-health-metrics.json"
)

func buildNamespace(ns string, isDownstream bool) *corev1.Namespace {
	labels := map[string]string{}
	if isDownstream {
		labels[downstreamLabelKey] = downstreamLabelValue
	}
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: labels,
		},
	}
}

func buildRoleMonitoringReader() *rbacv1.ClusterRole {
	cr := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.OperatorName + roleSuffix,
		},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""},
				Verbs:     []string{"get", "list", "watch"},
				Resources: []string{"pods", "services", "endpoints"},
			},
			{
				NonResourceURLs: []string{"/metrics"},
				Verbs:           []string{"get"},
			},
		},
	}
	return &cr
}

func buildRoleBindingMonitoringReader(ns string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.OperatorName + roleSuffix,
			Namespace: ns,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     constants.OperatorName + roleSuffix,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      monitoringServiceAccount,
			Namespace: monitoringNamespace,
		}},
	}
}

func buildFlowMetricsDashboard(namespace string, ignoreFlags []string) (*corev1.ConfigMap, bool, error) {
	dashboard, err := helper.CreateFlowMetricsDashboard(namespace, ignoreFlags)
	if err != nil {
		return nil, false, err
	}

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      flowDashboardCMName,
			Namespace: dashboardCMNamespace,
			Labels: map[string]string{
				dashboardCMAnnotation: "true",
			},
		},
		Data: map[string]string{
			flowDashboardCMFile: dashboard,
		},
	}
	return &configMap, len(dashboard) == 0, nil
}

func buildHealthDashboard(ignoreFlags []string) (*corev1.ConfigMap, bool, error) {
	dashboard, err := helper.FilterDashboardRows(healthDashboardEmbed, ignoreFlags)
	if err != nil {
		return nil, false, err
	}

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      healthDashboardCMName,
			Namespace: dashboardCMNamespace,
			Labels: map[string]string{
				dashboardCMAnnotation: "true",
			},
		},
		Data: map[string]string{
			healthDashboardCMFile: dashboard,
		},
	}
	return &configMap, len(dashboard) == 0, nil
}
