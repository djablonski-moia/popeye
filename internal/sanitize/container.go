package sanitize

import (
	"strings"

	"github.com/derailed/popeye/internal/annotation_config"
	"github.com/derailed/popeye/internal/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Docker image latest tag.
const imageTagLatest = "latest"

type (
	// LimitCollector represents a collector with resource limits.
	LimitCollector interface {
		Collector
		PodLimiter
	}

	// Container represents a Container linter.
	Container struct {
		LimitCollector
		fqn string
	}
)

// NewContainer returns a new container linter.
func NewContainer(fqn string, c LimitCollector) *Container {
	return &Container{fqn: fqn, LimitCollector: c}
}

// Lint a Container.
func (c *Container) sanitize(meta *metav1.ObjectMeta, co v1.Container, checkProbes bool) {
	skipCodes := annotation_config.NewSkipAnnotationConfig(meta)

	c.checkImageTags(skipCodes, co.Name, co.Image)
	c.checkResources(skipCodes, co)
	if checkProbes {
		c.checkProbes(skipCodes, co)
	}
	c.checkNamedPorts(skipCodes, co)
}

func (c *Container) checkImageTags(skipCodes annotation_config.SkipAnnotationConfig, name, image string) {
	tokens := strings.Split(image, ":")
	if len(tokens) < 2 {
		c.AddSubCodeWithSkipCheck(skipCodes,100, c.fqn, name)
		return
	}

	if tokens[1] == imageTagLatest {
		c.AddSubCodeWithSkipCheck(skipCodes, 101, c.fqn, name)
	}
}

func (c *Container) checkProbes(skipCodes annotation_config.SkipAnnotationConfig, co v1.Container) {
	if (co.LivenessProbe == nil && co.ReadinessProbe == nil) {
		c.AddSubCodeWithSkipCheck(skipCodes, 102, c.fqn, co.Name)
		return
	}

	if co.LivenessProbe == nil {
		c.AddSubCodeWithSkipCheck(skipCodes, 103, c.fqn, co.Name)
	}
	c.checkNamedProbe(skipCodes, co.Name, co.LivenessProbe, true)

	if co.ReadinessProbe == nil  {
		c.AddSubCodeWithSkipCheck(skipCodes, 104, c.fqn, co.Name)
	}
	c.checkNamedProbe(skipCodes, co.Name, co.ReadinessProbe, false)
}

func (c *Container) checkNamedProbe(skipCodes annotation_config.SkipAnnotationConfig, co string, p *v1.Probe, liveness bool) {
	if p == nil || p.Handler.HTTPGet == nil {
		return
	}

	kind := "Readiness"
	if liveness {
		kind = "Liveness"
	}
	if p.Handler.HTTPGet != nil && p.Handler.HTTPGet.Port.Type == intstr.Int {
		c.AddSubCodeWithSkipCheck(skipCodes, 105, c.fqn, co, kind)
	}
}

func (c *Container) checkResources(skipCodes annotation_config.SkipAnnotationConfig, co v1.Container) {
	if len(co.Resources.Limits) == 0 && len(co.Resources.Requests) == 0 {
		c.AddSubCodeWithSkipCheck(skipCodes, 106, c.fqn, co.Name)
		return
	}

	if len(co.Resources.Requests) > 0 && len(co.Resources.Limits) == 0 {
		c.AddSubCodeWithSkipCheck(skipCodes, 107, c.fqn, co.Name)
	}
}

func (c *Container) checkNamedPorts(skipCodes annotation_config.SkipAnnotationConfig, co v1.Container) {
	for _, p := range co.Ports {
		if len(p.Name) == 0 {
			c.AddSubCodeWithSkipCheck(skipCodes, 108, c.fqn, co.Name, p.ContainerPort)
		}
	}
}

func (c *Container) checkUtilization(co v1.Container, cmx k8s.Metrics) {
	cpu, mem, qos := containerResources(co)
	if cpu != nil && mem != nil {
		c.checkMetrics(qos, co.Name, *cpu, *mem, cmx.CurrentCPU, cmx.CurrentMEM)
	}
}

func (c *Container) checkMetrics(qos qos, co string, cpu, mem, ccpu, cmem resource.Quantity) {
	percCPU, cpuLimit := ToPerc(toMC(ccpu), toMC(cpu)), int64(c.PodCPULimit())
	percMEM, memLimit := ToPerc(toMB(cmem), toMB(mem)), int64(c.PodMEMLimit())

	switch qos {
	case qosBurstable:
		if percCPU > cpuLimit {
			c.AddSubCode(109, c.fqn, co, asMC(ccpu), asMC(cpu), cpuLimit, percCPU)
		}
		if percMEM > memLimit {
			c.AddSubCode(110, c.fqn, co, asMB(cmem), asMB(mem), memLimit, percMEM)
		}
	case qosGuaranteed:
		if percCPU > cpuLimit {
			c.AddSubCode(111, c.fqn, co, asMC(ccpu), asMC(cpu), cpuLimit, percCPU)
		}
		if percMEM > memLimit {
			c.AddSubCode(112, c.fqn, co, asMB(cmem), asMB(mem), memLimit, percMEM)
		}
	}
}
