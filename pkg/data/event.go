package data

import (
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

// Event types.
const (
	BottlePulled  = "BottlePulled"
	BottlePulling = "BottlePulling"
	BottleFailed  = "BottleFailed"
)

func getPodReference(volumeContext map[string]string) runtime.Object {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volumeContext["csi.storage.k8s.io/pod.name"],
			Namespace: volumeContext["csi.storage.k8s.io/pod.namespace"],
			UID:       types.UID(volumeContext["csi.storage.k8s.io/pod.uid"]),
		},
	}
	return pod
}
func createEventRecorder(log *slog.Logger, nodeid string) record.EventRecorder {
	// adapted from https://github.com/box/error-reporting-with-kubernetes-events/blob/master/cmd/controlplane/main.go#L201
	kubeConfig, err := restclient.InClusterConfig()
	if err != nil {
		log.Error("Could not get kubeconfig", "error", err)
		return nil
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Error("Error building kubernetes clientset", "error", err)

		return nil
	}

	eventBroadcaster := record.NewBroadcaster()
	// eventBroadcaster.StartLogging(func(format string, args ...any) { log.Info(fmt.Sprintf(format, args...)) })
	eventBroadcaster.StartRecordingToSink(
		&typedcorev1.EventSinkImpl{
			Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(
		scheme.Scheme,
		corev1.EventSource{Host: nodeid, Component: "csi-bottle"})
	return recorder
}
