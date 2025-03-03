package flowlogspipeline

import (
	appsv1 "k8s.io/api/apps/v1"
	ascv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netobserv/flowlogs-pipeline/pkg/api"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	flowslatest "github.com/netobserv/network-observability-operator/api/v1beta1"
	"github.com/netobserv/network-observability-operator/controllers/reconcilers"
	"github.com/netobserv/network-observability-operator/pkg/helper"
)

type transfoBuilder struct {
	generic builder
}

func newTransfoBuilder(info *reconcilers.Instance, desired *flowslatest.FlowCollectorSpec) transfoBuilder {
	gen := newBuilder(info, desired, ConfKafkaTransformer)
	return transfoBuilder{
		generic: gen,
	}
}

func (b *transfoBuilder) deployment(annotations map[string]string) *appsv1.Deployment {
	pod := b.generic.podTemplate(false /*no listen*/, false /*no host network*/, annotations)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.generic.name(),
			Namespace: b.generic.info.Namespace,
			Labels:    b.generic.labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: b.generic.desired.Processor.KafkaConsumerReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: b.generic.selector,
			},
			Template: pod,
		},
	}
}

func (b *transfoBuilder) configMap() (*corev1.ConfigMap, string, error) {
	stages, params, err := b.buildPipelineConfig()
	if err != nil {
		return nil, "", err
	}
	configMap, digest, err := b.generic.configMap(stages, params)
	return configMap, digest, err
}

func (b *transfoBuilder) buildPipelineConfig() ([]config.Stage, []config.StageParam, error) {
	// TODO in a later optimization patch: set ingester <-> transformer communication also via protobuf
	// For now, we leave this communication via JSON and just setup protobuf ingestion when
	// the transformer is communicating directly via eBPF agent
	decoder := api.Decoder{Type: "protobuf"}
	if helper.UseIPFIX(b.generic.desired) {
		decoder = api.Decoder{Type: "json"}
	}
	pipeline := config.NewKafkaPipeline("kafka-read", api.IngestKafka{
		Brokers:           []string{b.generic.desired.Kafka.Address},
		Topic:             b.generic.desired.Kafka.Topic,
		GroupId:           b.generic.name(), // Without groupid, each message is delivered to each consumers
		Decoder:           decoder,
		TLS:               b.generic.getKafkaTLS(&b.generic.desired.Kafka.TLS, "kafka-cert"),
		SASL:              b.generic.getKafkaSASL(&b.generic.desired.Kafka.SASL, "kafka-ingest"),
		PullQueueCapacity: b.generic.desired.Processor.KafkaConsumerQueueCapacity,
		PullMaxBytes:      b.generic.desired.Processor.KafkaConsumerBatchSize,
	})

	err := b.generic.addTransformStages(&pipeline)
	if err != nil {
		return nil, nil, err
	}
	return pipeline.GetStages(), pipeline.GetStageParams(), nil
}

func (b *transfoBuilder) promService() *corev1.Service {
	return b.generic.promService()
}

func (b *transfoBuilder) autoScaler() *ascv2.HorizontalPodAutoscaler {
	return &ascv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.generic.name(),
			Namespace: b.generic.info.Namespace,
			Labels:    b.generic.labels,
		},
		Spec: ascv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: ascv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       b.generic.name(),
			},
			MinReplicas: b.generic.desired.Processor.KafkaConsumerAutoscaler.MinReplicas,
			MaxReplicas: b.generic.desired.Processor.KafkaConsumerAutoscaler.MaxReplicas,
			Metrics:     b.generic.desired.Processor.KafkaConsumerAutoscaler.Metrics,
		},
	}
}

// The operator needs to have at least the same permissions as flowlogs-pipeline in order to grant them
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=create;delete;patch;update;get;watch;list
//+kubebuilder:rbac:groups=core,resources=pods;services;nodes,verbs=get;list;watch

func buildClusterRoleTransformer() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name(ConfKafkaTransformer),
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Verbs:     []string{"list", "get", "watch"},
			Resources: []string{"pods", "services", "nodes"},
		}, {
			APIGroups: []string{"apps"},
			Verbs:     []string{"list", "get", "watch"},
			Resources: []string{"replicasets"},
		}, {
			APIGroups: []string{"autoscaling"},
			Verbs:     []string{"create", "delete", "patch", "update", "get", "watch", "list"},
			Resources: []string{"horizontalpodautoscalers"},
		}},
	}
}

func (b *transfoBuilder) serviceAccount() *corev1.ServiceAccount {
	return b.generic.serviceAccount()
}

func (b *transfoBuilder) clusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return b.generic.clusterRoleBinding(ConfKafkaTransformer, false)
}
