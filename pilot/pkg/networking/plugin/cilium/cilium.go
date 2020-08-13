package cilium

import (
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	http_conn "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/networking"
	"istio.io/istio/pilot/pkg/networking/plugin"
	"istio.io/istio/pilot/pkg/networking/util"
)

const (
	CiliumBpfMetadata = "cilium.bpf_metadata"
	CiliumL7Policy    = "cilium.l7policy"
)

// Plugin is a Cilium plugin.
type Plugin struct{}

// NewPlugin returns an instance of the Cilium plugin
func NewPlugin() plugin.Plugin {
	return Plugin{}
}

// OnOutboundListener is called whenever a new outbound listener is added to the LDS output for a given service.
// Can be used to add additional filters on the outbound path.
func (Plugin) OnOutboundListener(in *plugin.InputParams, mutable *networking.MutableObjects) error {
	return configureListener(false, in, mutable)
}

// OnInboundListener is called whenever a new listener is added to the LDS output for a given service
// Can be used to add additional filters.
func (Plugin) OnInboundListener(in *plugin.InputParams, mutable *networking.MutableObjects) error {
	return configureListener(true, in, mutable)
}

// OnInboundPassthrough is called whenever a new passthrough filter chain is added to the LDS output.
func (Plugin) OnInboundPassthrough(in *plugin.InputParams, mutable *networking.MutableObjects) error {
	return nil
}

// InboundMTLSConfiguration configures the mTLS configuration for inbound listeners.
func (Plugin) InboundMTLSConfiguration(in *plugin.InputParams, passthrough bool) []plugin.MTLSSettings {
       return nil
}

func configureListener(ingress bool, in *plugin.InputParams, mutable *networking.MutableObjects) error {
	node := in.Node
	if node.Type != model.SidecarProxy {
		// Only care about sidecar.
		return nil
	}

	// We will lazily build filters for tcp/http as needed
	httpBuilt := false
	var httpFilters []*http_conn.HttpFilter

	for i := range mutable.FilterChains {
		switch mutable.FilterChains[i].ListenerProtocol {
		case networking.ListenerProtocolHTTP:
			if !httpBuilt {
				httpFilters = buildHTTP()
				httpBuilt = true
			}
			mutable.FilterChains[i].HTTP = append(mutable.FilterChains[i].HTTP, httpFilters...)
		}
	}

	// Add Cilium listener filter if http filters were injected
	if httpBuilt {
		ciliumListenerCfg := &BpfMetadata{IsIngress: ingress}
		listenerFilter := &listener.ListenerFilter{Name: CiliumBpfMetadata}
		listenerFilter.ConfigType = &listener.ListenerFilter_TypedConfig{TypedConfig: util.MessageToAny(ciliumListenerCfg)}
		mutable.Listener.ListenerFilters = append(mutable.Listener.ListenerFilters, listenerFilter)
	}

	return nil
}

func buildHTTP() []*http_conn.HttpFilter {
	ciliumHttpCfg := &L7Policy{AccessLogPath: "/var/run/cilium/access_log.sock"}
	httpFilter := &http_conn.HttpFilter{Name: CiliumL7Policy}
	httpFilter.ConfigType = &http_conn.HttpFilter_TypedConfig{TypedConfig: util.MessageToAny(ciliumHttpCfg)}

	return []*http_conn.HttpFilter{httpFilter}
}
