package membership

import (
	"context"
	"log"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type MembershipManager struct {
	ctx    context.Context
	client *kubernetes.Clientset
	mu     sync.Mutex
	Active map[string]struct{}
}

func NewMembershipManager(ctx context.Context) (*MembershipManager, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &MembershipManager{
		client: client,
		ctx:    ctx,
		Active: make(map[string]struct{}),
	}, nil
}

// WatchControllers now uses an informer instead of a direct watch
func (m *MembershipManager) WatchControllers(labelSelector string, onDelete func(string)) error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		m.client,
		30*time.Second, // resync period
		informers.WithNamespace("default"),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = labelSelector
		}),
	)

	podInformer := factory.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			m.mu.Lock()
			defer m.mu.Unlock()

			m.Active[pod.Name] = struct{}{}
			log.Printf("[Membership] Pod added: %s", pod.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*v1.Pod)
			m.mu.Lock()
			defer m.mu.Unlock()

			if newPod.Status.Phase == v1.PodFailed || newPod.Status.Phase == v1.PodSucceeded {
				delete(m.Active, newPod.Name)
				log.Printf("[Membership] Pod updated to terminated state: %s", newPod.Name)
				onDelete(newPod.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			m.mu.Lock()
			defer m.mu.Unlock()

			delete(m.Active, pod.Name)
			log.Printf("[Membership] Pod deleted: %s", pod.Name)
			onDelete(pod.Name)
		},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)

	log.Printf("[Membership] Starting informer for Pods with selector: %s", labelSelector)
	factory.Start(stopCh)

	// Wait for cache sync before processing events
	if !cache.WaitForCacheSync(m.ctx.Done(), podInformer.HasSynced) {
		return context.Canceled
	}

	// Block until context is canceled
	<-m.ctx.Done()
	log.Printf("[Membership] Context canceled, stopping informer.")
	return nil
}
