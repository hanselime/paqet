//go:build ignore

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/pkt_cls.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>

#ifdef BPF_PRINTK_UNSUPPORTED
#undef bpf_printk
#define bpf_printk(...)
#endif

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 65535);
    __type(key, __u32);
    __type(value, __u16);
} target_port SEC(".maps");

/*
 * tcp to udp conversion scheme:
 *
 * original TCP packet arrives:
 *
 *  +--------+--------+--------+-------------------+
 *  | ETH hdr| IP hdr | TCP hdr| TCP payload       |
 *  +--------+--------+--------+-------------------+
 *
 * after eBPF conversion:
 *
 *  +--------+--------+--------+--------+----------+
 *  | ETH hdr| IP hdr | UDP hdr| off(2B)| TCP payload
 *  +--------+--------+--------+--------+----------+
 *
 * - 'off' field indicates offset of actual payload in userspace
 * - done in kernel to reduce context switches and enable a true zero-copy
 */
struct paqethdr {
	struct udphdr udphdr;
	__be16 off;
};

struct cursor {
	void *pos;
};

static __always_inline int parse_ethhdr(struct cursor *nh,
					void *data_end,
					struct ethhdr **ethhdr)
{
	struct ethhdr *eth = nh->pos;
	int hdrsize = sizeof(*eth);

	if ((void *)eth + hdrsize > data_end)
		return -1;

	nh->pos += hdrsize;
	*ethhdr = eth;

	return eth->h_proto;
}

static __always_inline int parse_ip6hdr(struct cursor *nh,
					void *data_end,
					struct ipv6hdr **ip6hdr)
{
	struct ipv6hdr *ip6h = nh->pos;
	int hdrsize = sizeof(*ip6h);

	if ((void *)ip6h + hdrsize > data_end)
		return -1;

	nh->pos += hdrsize;
	*ip6hdr = ip6h;

	return ip6h->nexthdr;
}

static __always_inline int parse_iphdr(struct cursor *nh,
				       void *data_end,
				       struct iphdr **iphdr)
{
	struct iphdr *iph = nh->pos;
	int hdrsize = sizeof(*iph);

	if ((void *)iph + hdrsize > data_end)
		return -1;

	hdrsize = iph->ihl << 2;
	if(hdrsize < sizeof(*iph))
		return -1;

	if (nh->pos + hdrsize > data_end)
		return -1;

	nh->pos += hdrsize;
	*iphdr = iph;

	return iph->protocol;
}

static __always_inline int parse_tcphdr(struct cursor *nh,
					void *data_end,
					struct tcphdr **tcphdr)
{
	struct tcphdr *tcph = nh->pos;
	int hdrsize = sizeof(*tcph);

	if ((void *)tcph + hdrsize > data_end)
		return -1;

	hdrsize = tcph->doff << 2;
	if(hdrsize < sizeof(*tcph))
		return -1;

	if (nh->pos + hdrsize > data_end)
		return -1;

	nh->pos += hdrsize;
	*tcphdr = tcph;

	return hdrsize;
}

static __always_inline int parse_udphdr(struct cursor *nh,
					void *data_end,
					struct udphdr **udphdr)
{
	struct udphdr *udph = nh->pos;
	int hdrsize = sizeof(*udph);
	int len;

	if ((void *)udph + hdrsize > data_end)
		return -1;

	nh->pos += hdrsize;
	*udphdr = udph;

	len = bpf_ntohs(udph->len) - hdrsize;
	if (len < 0)
		return -1;

	return len;
}

static __always_inline int
tcp_to_udp(struct __sk_buff *skb, struct cursor *nh,
	   struct iphdr *iphdr, struct ipv6hdr *ipv6hdr)
{
	void *data_end = (void *)(long)skb->data_end;
	void *data = (void *)(long)skb->data;
	struct paqethdr *pqhdr = nh->pos;
	struct tcphdr *tcphdr, tcphdr_cpy;
	int nh_off = nh->pos - data;
	__be16 udp_len, zero = 0;
	__be16 proto_old = bpf_htons(IPPROTO_TCP);
	__be16 proto_new = bpf_htons(IPPROTO_UDP);

	if (parse_tcphdr(nh, data_end, &tcphdr) < 0)
		goto out;

	__u32 port = bpf_htons(tcphdr->dest);
	__u16 *found = bpf_map_lookup_elem(&target_port, &port);
	if (!(found && *found == 1))
		return TC_ACT_OK;

	if (iphdr) {
		udp_len = bpf_htons(bpf_ntohs(iphdr->tot_len) -
				    ((void*)tcphdr - (void*)iphdr));
	} else if (ipv6hdr) {
		udp_len = ipv6hdr->payload_len;
	} else {
		goto out;
	}

	pqhdr->udphdr.check = tcphdr->check;
	pqhdr->udphdr.len = udp_len;
	pqhdr->off = bpf_htons((tcphdr->doff << 2) - sizeof(struct udphdr));

	if (iphdr) {
		int ip_off = (void*)iphdr - data;
		iphdr->protocol = IPPROTO_UDP;

		bpf_l3_csum_replace(skb, ip_off + offsetof(struct iphdr, check),
				    proto_old, proto_new, sizeof(__be16));
	} else if (ipv6hdr) {
		ipv6hdr->nexthdr = IPPROTO_UDP;
	}
	bpf_l4_csum_replace(skb, nh_off + offsetof(struct udphdr, check),
			    proto_old, proto_new, sizeof(__be16) | BPF_F_PSEUDO_HDR);

	bpf_l4_csum_replace(skb, nh_off + offsetof(struct udphdr, check),
			    zero, udp_len, sizeof(__be16));

	return TC_ACT_PIPE;
out:
	return TC_ACT_OK;
}

SEC("tc")
int tc_tcp_to_paqet(struct __sk_buff *skb)
{
	void *data_end = (void *)(long)skb->data_end;
	void *data = (void *)(long)skb->data;
	struct cursor nh = { .pos = data };
	int eth_type, ip_type;
	struct ipv6hdr *ipv6hdr = NULL;
	struct iphdr *iphdr = NULL;
	struct ethhdr *eth;

	eth_type = parse_ethhdr(&nh, data_end, &eth);
	if (eth_type == bpf_htons(ETH_P_IP))
		ip_type = parse_iphdr(&nh, data_end, &iphdr);
	else if (eth_type == bpf_htons(ETH_P_IPV6))
		ip_type = parse_ip6hdr(&nh, data_end, &ipv6hdr);
	else
		goto out;

	if (ip_type == IPPROTO_TCP)
		return tcp_to_udp(skb, &nh, iphdr, ipv6hdr);
out:
	return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
