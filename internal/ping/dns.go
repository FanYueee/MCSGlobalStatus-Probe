package ping

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

type DnsRecord struct {
	Hostname string `json:"hostname"`
	Type     string `json:"type"`
	Data     string `json:"data"`
}

type SrvRecord struct {
	Target string `json:"target"`
	Port   int    `json:"port"`
}

type IpInfo struct {
	IP         string      `json:"ip,omitempty"`
	IPs        []string    `json:"ips,omitempty"`
	SrvRecord  *SrvRecord  `json:"srv_record,omitempty"`
	DNSRecords []DnsRecord `json:"dns_records,omitempty"`
}

type dnsSnapshot struct {
	ConnectHost string
	ConnectPort int
	IPInfo      *IpInfo
}

func resolveJavaSnapshot(host string, port int, timeout time.Duration) (*dnsSnapshot, error) {
	if net.ParseIP(host) != nil {
		return &dnsSnapshot{
			ConnectHost: host,
			ConnectPort: port,
			IPInfo: &IpInfo{
				IP: host,
			},
		}, nil
	}

	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	records := make([]DnsRecord, 0, 4)
	ipInfo := &IpInfo{}
	connectHost := host
	connectPort := port

	_, srvAddrs, err := resolver.LookupSRV(ctx, "minecraft", "tcp", host)
	if err == nil && len(srvAddrs) > 0 {
		target := strings.TrimSuffix(srvAddrs[0].Target, ".")
		connectHost = target
		connectPort = int(srvAddrs[0].Port)
		ipInfo.SrvRecord = &SrvRecord{
			Target: target,
			Port:   connectPort,
		}
		records = append(records, DnsRecord{
			Hostname: "_minecraft._tcp." + host,
			Type:     "SRV",
			Data:     fmt.Sprintf("1 1 %d %s", connectPort, target),
		})
	}

	if err := resolveHostSnapshot(ctx, resolver, connectHost, ipInfo, &records); err != nil {
		if len(records) > 0 {
			ipInfo.DNSRecords = records
		}
		return &dnsSnapshot{
			ConnectHost: connectHost,
			ConnectPort: connectPort,
			IPInfo:      ipInfo,
		}, err
	}

	return &dnsSnapshot{
		ConnectHost: connectHostFromInfo(connectHost, ipInfo),
		ConnectPort: connectPort,
		IPInfo:      ipInfo,
	}, nil
}

func resolveBedrockSnapshot(host string, port int, timeout time.Duration) (*dnsSnapshot, error) {
	if net.ParseIP(host) != nil {
		return &dnsSnapshot{
			ConnectHost: host,
			ConnectPort: port,
			IPInfo: &IpInfo{
				IP: host,
			},
		}, nil
	}

	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	records := make([]DnsRecord, 0, 3)
	ipInfo := &IpInfo{}

	if err := resolveHostSnapshot(ctx, resolver, host, ipInfo, &records); err != nil {
		if len(records) > 0 {
			ipInfo.DNSRecords = records
		}
		return &dnsSnapshot{
			ConnectHost: host,
			ConnectPort: port,
			IPInfo:      ipInfo,
		}, err
	}

	return &dnsSnapshot{
		ConnectHost: connectHostFromInfo(host, ipInfo),
		ConnectPort: port,
		IPInfo:      ipInfo,
	}, nil
}

func resolveHostSnapshot(
	ctx context.Context,
	resolver *net.Resolver,
	host string,
	ipInfo *IpInfo,
	records *[]DnsRecord,
) error {
	if net.ParseIP(host) != nil {
		ipInfo.IP = host
		return nil
	}

	canonical, err := resolver.LookupCNAME(ctx, host)
	targetHost := host
	if err == nil {
		canonical = strings.TrimSuffix(canonical, ".")
		if canonical != "" && !strings.EqualFold(canonical, host) {
			targetHost = canonical
			*records = append(*records, DnsRecord{
				Hostname: host,
				Type:     "CNAME",
				Data:     canonical,
			})
		}
	}

	ips, err := resolver.LookupIPAddr(ctx, targetHost)
	if err != nil || len(ips) == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("DNS resolution failed")
	}

	uniqueIPs := make([]string, 0, len(ips))
	seen := make(map[string]struct{}, len(ips))

	for _, addr := range ips {
		ipStr := addr.IP.String()
		if _, ok := seen[ipStr]; ok {
			continue
		}
		seen[ipStr] = struct{}{}
		uniqueIPs = append(uniqueIPs, ipStr)

		recordType := "AAAA"
		if addr.IP.To4() != nil {
			recordType = "A"
		}

		*records = append(*records, DnsRecord{
			Hostname: targetHost,
			Type:     recordType,
			Data:     ipStr,
		})
	}

	if len(uniqueIPs) == 0 {
		return fmt.Errorf("DNS resolution failed")
	}

	ipInfo.IP = preferIPv4(uniqueIPs)
	if len(uniqueIPs) > 1 {
		ipInfo.IPs = uniqueIPs
	}
	if len(*records) > 0 {
		ipInfo.DNSRecords = *records
	}

	return nil
}

func connectHostFromInfo(fallbackHost string, ipInfo *IpInfo) string {
	if ipInfo != nil && ipInfo.IP != "" {
		return ipInfo.IP
	}
	return fallbackHost
}

func preferIPv4(ips []string) string {
	for _, ip := range ips {
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			return ip
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}
