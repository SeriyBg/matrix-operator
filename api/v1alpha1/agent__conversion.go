package v1alpha1

import (
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/SeriyBg/matrix-operator/api/v1beta1"
)

// ConvertTo converts this Memcached to the Hub version (vbeta1).
func (src *Agent) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.Agent)
	dst.ObjectMeta = src.ObjectMeta
	agents := make([]v1beta1.SingleAgent, 0, len(src.Spec.Names))
	for _, name := range src.Spec.Names {
		agents = append(agents, v1beta1.SingleAgent{Name: v1beta1.AgentName(name), Health: pointer.Int32(100)})
	}
	dst.Spec.Agents = agents
	return nil
}

// ConvertFrom converts from the Hub version (vbeta1) to this version.
func (dst *Agent) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.Agent)
	dst.ObjectMeta = src.ObjectMeta
	names := make([]AgentNames, 0, len(src.Spec.Agents))
	for _, agent := range src.Spec.Agents {
		names = append(names, AgentNames(agent.Name))
	}
	dst.Spec.Names = names
	return nil
}
