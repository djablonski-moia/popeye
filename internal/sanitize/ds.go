package sanitize

import (
	"context"
	"errors"

	"github.com/derailed/popeye/internal/issues"
	"github.com/derailed/popeye/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

type (
	// DaemonSet tracks DaemonSet sanitization.
	DaemonSet struct {
		*issues.Collector
		DaemonSetLister
	}

	// DaemonLister list DaemonSets.
	DaemonLister interface {
		ListDaemonSets() map[string]*appsv1.DaemonSet
	}

	// DaemonSetLister list available DaemonSets on a cluster.
	DaemonSetLister interface {
		PodLimiter
		PodsMetricsLister
		PodSelectorLister
		ConfigLister
		DaemonLister
	}
)

// NewDaemonSet returns a new DaemonSet sanitizer.
func NewDaemonSet(co *issues.Collector, lister DaemonSetLister) *DaemonSet {
	return &DaemonSet{
		Collector:       co,
		DaemonSetLister: lister,
	}
}

// Sanitize configmaps.
func (d *DaemonSet) Sanitize(ctx context.Context) error {
	over := pullOverAllocs(ctx)
	for fqn, ds := range d.ListDaemonSets() {
		d.InitOutcome(fqn)
		d.checkDeprecation(fqn, ds)
		d.checkContainers(fqn, ds.Spec.Template)
		pmx := k8s.PodsMetrics{}
		podsMetrics(d, pmx)

		d.checkUtilization(over, fqn, ds, pmx)
	}

	return nil
}

func (d *DaemonSet) checkDeprecation(fqn string, ds *appsv1.DaemonSet) {
	const current = "apps/v1"

	rev, err := resourceRev(fqn, ds.Annotations)
	if err != nil {
		rev = revFromLink(ds.SelfLink)
		if rev == "" {
			d.AddCode(404, fqn, errors.New("Unable to assert resource version"))
			return
		}
	}
	if rev != current {
		d.AddCode(403, fqn, "DaemonSet", rev, current)
	}
}

// CheckContainers runs thru deployment template and checks pod configuration.
func (d *DaemonSet) checkContainers(fqn string, spec v1.PodTemplateSpec) {
	c := NewContainer(fqn, d)
	for _, co := range spec.Spec.InitContainers {
		c.sanitize(&spec.ObjectMeta, co, false)
	}
	for _, co := range spec.Spec.Containers {
		c.sanitize(&spec.ObjectMeta, co, false)
	}
}

// CheckUtilization checks deployments requested resources vs current utilization.
func (d *DaemonSet) checkUtilization(over bool, fqn string, ds *appsv1.DaemonSet, pmx k8s.PodsMetrics) error {
	mx := d.daemonsetUsage(ds, pmx)
	if mx.RequestCPU.IsZero() && mx.RequestMEM.IsZero() {
		return nil
	}

	cpuPerc := mx.ReqCPURatio()
	if cpuPerc > 1 && cpuPerc > float64(d.CPUResourceLimits().UnderPerc) {
		d.AddCode(503, fqn, asMC(mx.CurrentCPU), asMC(mx.RequestCPU), asPerc(cpuPerc))
	} else if over && cpuPerc < float64(d.CPUResourceLimits().OverPerc) {
		d.AddCode(504, fqn, asMC(mx.CurrentCPU), asMC(mx.RequestCPU), asPerc(mx.ReqAbsCPURatio()))
	}

	memPerc := mx.ReqMEMRatio()
	if memPerc > 1 && memPerc > float64(d.MEMResourceLimits().UnderPerc) {
		d.AddCode(505, fqn, asMB(mx.CurrentMEM), asMB(mx.RequestMEM), asPerc(memPerc))
	} else if over && memPerc < float64(d.MEMResourceLimits().OverPerc) {
		d.AddCode(506, fqn, asMB(mx.CurrentMEM), asMB(mx.RequestMEM), asPerc(mx.ReqAbsMEMRatio()))
	}

	return nil
}

// DaemonSetUsage finds deployment running pods and compute current vs requested resource usage.
func (d *DaemonSet) daemonsetUsage(ds *appsv1.DaemonSet, pmx k8s.PodsMetrics) ConsumptionMetrics {
	var mx ConsumptionMetrics
	for pfqn, pod := range d.ListPodsBySelector(ds.Spec.Selector) {
		cpu, mem := computePodResources(pod.Spec)
		mx.QOS = pod.Status.QOSClass
		mx.RequestCPU.Add(cpu)
		mx.RequestMEM.Add(mem)

		ccx, ok := pmx[pfqn]
		if !ok {
			continue
		}
		for _, cx := range ccx {
			mx.CurrentCPU.Add(cx.CurrentCPU)
			mx.CurrentMEM.Add(cx.CurrentMEM)
		}
	}

	return mx
}
