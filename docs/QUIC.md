# QUIC Protocol Support

paqet now supports QUIC as an alternative transport protocol to KCP. QUIC is optimized for high bandwidth scenarios and can handle many concurrent connections efficiently.

## Why QUIC?

QUIC (Quick UDP Internet Connections) offers several advantages:

- **Faster Connection Establishment**: 0-RTT support allows reconnections without handshake overhead
- **Better Multiplexing**: Native stream multiplexing without head-of-line blocking
- **Improved Congestion Control**: Modern congestion control algorithms
- **Built-in Encryption**: TLS 1.3 integrated into the protocol
- **Connection Migration**: Connections can survive IP address changes
- **Optimized for High Bandwidth**: Large receive windows and efficient flow control

## QUIC vs KCP

| Feature | QUIC | KCP |
|---------|------|-----|
| Connection Setup | 0-RTT (faster) | Custom handshake |
| Encryption | TLS 1.3 (built-in) | Custom cipher support |
| Stream Multiplexing | Native (no head-of-line blocking) | Via SMUX library |
| Congestion Control | Modern (BBR-like) | Configurable (aggressive modes) |
| Standard Compliance | IETF standard | Custom protocol |
| Best For | High bandwidth, many connections | Low latency, lossy networks |

## Configuration

### Basic Setup

To use QUIC, simply set `protocol: "quic"` in your transport configuration:

**Client:**
```yaml
transport:
  protocol: "quic"
  quic:
    insecure_skip_verify: true  # Required for self-signed certs
```

**Server:**
```yaml
transport:
  protocol: "quic"
  quic:
    max_incoming_streams: 10000  # High limit for servers
```

### Performance Tuning for High Bandwidth

For maximum throughput on high-bandwidth connections:

```yaml
transport:
  protocol: "quic"
  quic:
    # Increase concurrent streams
    max_incoming_streams: 20000
    max_incoming_uni_streams: 20000
    
    # Maximize flow control windows
    initial_stream_receive_window: 10485760      # 10 MB
    max_stream_receive_window: 41943040          # 40 MB
    initial_connection_receive_window: 26214400  # 25 MB
    max_connection_receive_window: 104857600     # 100 MB
    
    # Enable 0-RTT for fast reconnections
    enable_0rtt: true
    
    # Shorter keep-alive for faster failure detection
    keep_alive_period: 5
```

### Performance Tuning for Many Connections

For handling thousands of concurrent connections:

```yaml
transport:
  protocol: "quic"
  conn: 4  # Use multiple underlying connections
  quic:
    # Balance stream limits
    max_incoming_streams: 5000
    max_incoming_uni_streams: 5000
    
    # Moderate flow control (memory efficiency)
    initial_stream_receive_window: 6291456      # 6 MB
    max_stream_receive_window: 25165824         # 24 MB
    initial_connection_receive_window: 15728640 # 15 MB
    max_connection_receive_window: 62914560     # 60 MB
    
    # Connection management
    max_idle_timeout: 60
    keep_alive_period: 15
```

## Configuration Reference

### Connection Settings

- **`max_idle_timeout`** (default: 30): Maximum idle timeout in seconds (1-600)
  - How long a connection can be idle before being closed
  - Lower values free resources faster but may disconnect idle clients

- **`max_incoming_streams`** (default: 1000 client, 10000 server): Maximum concurrent bidirectional streams
  - Each forwarded connection or SOCKS5 request uses one stream
  - Set higher for servers handling many simultaneous requests

- **`max_incoming_uni_streams`** (default: 1000 client, 10000 server): Maximum concurrent unidirectional streams
  - Used for control messages and metadata
  - Generally matches or is slightly lower than `max_incoming_streams`

### Flow Control Settings

Flow control windows determine how much data can be in-flight before requiring acknowledgment.

- **`initial_stream_receive_window`** (default: 6 MB): Initial per-stream receive window
  - Starting buffer size for each stream
  - Larger values allow more data in-flight = higher throughput
  - Must be >= 1 MB

- **`max_stream_receive_window`** (default: 24 MB): Maximum per-stream receive window
  - Window can grow to this size based on congestion control
  - Should be 4-10x the initial window
  - Must be >= `initial_stream_receive_window`

- **`initial_connection_receive_window`** (default: 15 MB): Initial connection-wide receive window
  - Total buffer for all streams combined
  - Should be 2-3x `initial_stream_receive_window`
  - Must be >= 1 MB

- **`max_connection_receive_window`** (default: 60 MB): Maximum connection-wide receive window
  - Total buffer capacity across all streams
  - Should be 2-3x `max_stream_receive_window`
  - Must be >= `initial_connection_receive_window`

### Performance Settings

- **`enable_datagrams`** (default: false): Enable QUIC datagram support
  - Allows unreliable message delivery (like UDP)
  - Currently not used by paqet but available for future features

- **`enable_0rtt`** (default: true): Enable 0-RTT connection resumption
  - Subsequent connections to same server can skip handshake
  - Significantly reduces connection establishment latency
  - Recommended to keep enabled

- **`keep_alive_period`** (default: 10): Keep-alive period in seconds (1-60)
  - How often to send keep-alive packets on idle connections
  - Lower values detect failures faster but use more bandwidth
  - Balance based on network reliability

### TLS Settings (Client Only)

- **`insecure_skip_verify`** (default: false): Skip TLS certificate verification
  - Set to `true` when using self-signed certificates (testing/development)
  - Set to `false` in production with proper certificates
  - **Security Warning**: Only use `true` in trusted environments

- **`server_name`**: Server name for TLS verification
  - Override the server name used in TLS verification
  - By default uses the server's IP address
  - Useful when connecting to servers with domain certificates

## TLS and Certificates

### Server

The server **automatically generates** a self-signed certificate at startup. No manual certificate configuration is needed.

### Client

Clients must set `insecure_skip_verify: true` when connecting to servers with self-signed certificates:

```yaml
quic:
  insecure_skip_verify: true
```

For production deployments, you should:
1. Use proper certificates on the server (Let's Encrypt, etc.)
2. Set `insecure_skip_verify: false` on clients
3. Optionally specify `server_name` for domain-based verification

## Example Configurations

See the example configurations:
- `example/client-quic.yaml.example` - Client with QUIC
- `example/server-quic.yaml.example` - Server with QUIC

## Performance Benchmarks

### Compared to KCP

**High Bandwidth (100+ Mbps, low latency):**
- QUIC: ⚡ Faster (better congestion control)
- KCP: ✓ Good

**High Bandwidth (100+ Mbps, moderate latency 50-100ms):**
- QUIC: ⚡ Much Faster (0-RTT, efficient multiplexing)
- KCP: ✓ Good

**Many Concurrent Connections (1000+):**
- QUIC: ⚡ Excellent (native multiplexing)
- KCP: ✓ Good (SMUX overhead)

**High Packet Loss (>5%):**
- QUIC: ✓ Good (standard congestion control)
- KCP: ⚡ Better (aggressive retransmission modes)

**Very Low Latency (<10ms, gaming):**
- QUIC: ✓ Good
- KCP: ⚡ Better (fast2/fast3 modes)

### Recommendations

Use **QUIC** when:
- ✅ You need maximum throughput on high-bandwidth links
- ✅ You're handling many concurrent connections
- ✅ You want standards-based, well-tested protocol
- ✅ Connection setup time matters (0-RTT)
- ✅ Network latency is moderate to high (>20ms)

Use **KCP** when:
- ✅ You're on a very lossy network (>5% packet loss)
- ✅ You need the absolute lowest latency (<10ms)
- ✅ You want custom encryption options
- ✅ You're optimizing for real-time gaming or VoIP

## Troubleshooting

### Connection Failures

**Error: "TLS handshake failed"**
- Client: Ensure `insecure_skip_verify: true` when using self-signed certs
- Server: Check if the server is generating the certificate correctly

**Error: "connection timeout"**
- Check firewall rules (see below)
- Verify server address and port
- Ensure network interface settings are correct

### Firewall Configuration

QUIC still requires the same iptables rules as KCP because it uses raw packet injection:

```bash
sudo iptables -t raw -A PREROUTING -p tcp --dport 9999 -j NOTRACK
sudo iptables -t raw -A OUTPUT -p tcp --sport 9999 -j NOTRACK  
sudo iptables -t mangle -A OUTPUT -p tcp --sport 9999 --tcp-flags RST RST -j DROP
```

Replace `9999` with your actual port.

### Performance Issues

**Low throughput:**
1. Increase flow control windows (see "Performance Tuning for High Bandwidth")
2. Check for packet loss with `ping` or `mtr`
3. Monitor CPU usage - may need more `packet_workers` in performance config

**High latency:**
1. Reduce `keep_alive_period` for faster failure detection
2. Enable `enable_0rtt` if not already enabled
3. Check network path with `traceroute`

**Connection drops:**
1. Increase `max_idle_timeout` if connections are legitimately idle
2. Check firewall rules aren't interfering
3. Monitor server logs for errors

## Migration from KCP

To migrate an existing paqet setup from KCP to QUIC:

1. **Update configuration files:**
   ```yaml
   # Change this:
   transport:
     protocol: "kcp"
     kcp:
       # ... kcp settings ...
   
   # To this:
   transport:
     protocol: "quic"
     quic:
       insecure_skip_verify: true  # For self-signed certs
       # ... optional quic settings ...
   ```

2. **Restart server first**, then clients

3. **No changes needed to:**
   - Network interface configuration
   - SOCKS5/forward configuration
   - Performance settings
   - Firewall rules

4. **Test thoroughly** before deploying to production

## Future Enhancements

Planned features for QUIC support:
- [ ] QUIC datagram support for UDP optimization
- [ ] Custom certificate loading
- [ ] Connection migration support
- [ ] Detailed performance metrics
- [ ] BBR congestion control tuning

## References

- [QUIC Protocol (RFC 9000)](https://www.rfc-editor.org/rfc/rfc9000.html)
- [quic-go Library](https://github.com/quic-go/quic-go)
- [QUIC vs TCP Performance](https://blog.cloudflare.com/the-road-to-quic/)
