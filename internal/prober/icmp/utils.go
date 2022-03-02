// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package icmp

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"time"

	"github.com/go-kit/kit/log"       //nolint:staticcheck // TODO(mem): replace in BBE
	"github.com/go-kit/kit/log/level" //nolint:staticcheck // TODO(mem): replace in BBE

	"github.com/prometheus/client_golang/prometheus"
)

var protocolToGauge = map[string]float64{
	"ip4": 4,
	"ip6": 6,
}

// Returns the IP for the ipProtocol and lookup time.
func chooseProtocol(ctx context.Context, ipProtocol string, fallbackIPProtocol bool, target string, registry *prometheus.Registry, logger log.Logger) (ip *net.IPAddr, lookupTime float64, err error) {
	var fallbackProtocol string
	probeDNSLookupTimeSeconds := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_dns_lookup_time_seconds",
		Help: "Returns the time taken for probe dns lookup in seconds",
	})

	probeIPProtocolGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_ip_protocol",
		Help: "Specifies whether probe ip protocol is IP4 or IP6",
	})

	probeIPAddrHash := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_ip_addr_hash",
		Help: "Specifies the hash of IP address. It's useful to detect if the IP address changes.",
	})
	registry.MustRegister(probeIPProtocolGauge)
	registry.MustRegister(probeDNSLookupTimeSeconds)
	registry.MustRegister(probeIPAddrHash)

	if ipProtocol == "ip6" || ipProtocol == "" {
		ipProtocol = "ip6"
		fallbackProtocol = "ip4"
	} else {
		ipProtocol = "ip4"
		fallbackProtocol = "ip6"
	}

	_ = level.Info(logger).Log("msg", "Resolving target address", "ip_protocol", ipProtocol)
	resolveStart := time.Now()

	defer func() {
		lookupTime = time.Since(resolveStart).Seconds()
		probeDNSLookupTimeSeconds.Add(lookupTime)
	}()

	resolver := &net.Resolver{}
	if !fallbackIPProtocol {
		ips, err := resolver.LookupIP(ctx, ipProtocol, target)
		if err == nil {
			for _, ip := range ips {
				_ = level.Info(logger).Log("msg", "Resolved target address", "ip", ip.String())
				probeIPProtocolGauge.Set(protocolToGauge[ipProtocol])
				probeIPAddrHash.Set(ipHash(ip))
				return &net.IPAddr{IP: ip}, lookupTime, nil
			}
		}
		_ = level.Error(logger).Log("msg", "Resolution with IP protocol failed", "err", err)
		return nil, 0.0, err
	}

	ips, err := resolver.LookupIPAddr(ctx, target)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Resolution with IP protocol failed", "err", err)
		return nil, 0.0, err
	}

	// Return the IP in the requested protocol.
	fallbackIdx := int(-1)
	for i, ip := range ips {
		switch ipProtocol {
		case "ip4":
			if ip.IP.To4() != nil {
				_ = level.Info(logger).Log("msg", "Resolved target address", "ip", ip.String())
				probeIPProtocolGauge.Set(4)
				probeIPAddrHash.Set(ipHash(ip.IP))
				return &ip, lookupTime, nil
			}

			// ip4 as fallback
			fallbackIdx = i

		case "ip6":
			if ip.IP.To4() == nil {
				_ = level.Info(logger).Log("msg", "Resolved target address", "ip", ip.String())
				probeIPProtocolGauge.Set(6)
				probeIPAddrHash.Set(ipHash(ip.IP))
				return &ip, lookupTime, nil
			}

			// ip6 as fallback
			fallbackIdx = i
		}
	}

	// Unable to find ip and no fallback set.
	if fallbackIdx == -1 || !fallbackIPProtocol {
		return nil, 0.0, fmt.Errorf("unable to find ip; no fallback")
	}

	// Use fallback ip protocol.
	if fallbackProtocol == "ip4" {
		probeIPProtocolGauge.Set(4)
	} else {
		probeIPProtocolGauge.Set(6)
	}
	fallback := ips[fallbackIdx]
	probeIPAddrHash.Set(ipHash(fallback.IP))
	_ = level.Info(logger).Log("msg", "Resolved target address", "ip", fallback.String())
	return &fallback, lookupTime, nil
}

func ipHash(ip net.IP) float64 {
	h := fnv.New32a()
	_, _ = h.Write(ip)
	return float64(h.Sum32())
}
