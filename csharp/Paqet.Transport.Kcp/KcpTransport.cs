using System.Buffers.Binary;
using System.Net;
using System.Net.Sockets;
using System.Threading.Channels;
using KcpSharp;
using Paqet.Core;
using Paqet.Socket;

namespace Paqet.Transport.Kcp;

public sealed class KcpTransport : ITransport
{
    public async ValueTask<IConnection> DialAsync(Address address, CancellationToken cancellationToken = default)
    {
        var remote = new IPEndPoint(IPAddress.Parse(address.Host), address.Port);
        var localIp = ResolveLocalIPv4(remote.Address);
        var channel = new RawTcpPacketChannel(localIp, remote.Address, (ushort)remote.Port);
        var conv = (uint)Random.Shared.Next(1, int.MaxValue);
        var session = new KcpSession(conv, channel);
        session.Start();
        return await ValueTask.FromResult<IConnection>(new KcpConnection(session));
    }

    public async ValueTask<IListener> ListenAsync(Address address, CancellationToken cancellationToken = default)
    {
        var listener = new KcpListener(IPAddress.Parse(address.Host), (ushort)address.Port);
        listener.Start();
        return await ValueTask.FromResult<IListener>(listener);
    }

    private sealed class KcpListener : IListener
    {
        private readonly IPAddress _listenAddress;
        private readonly ushort _listenPort;
        private readonly Channel<IConnection> _accept = Channel.CreateUnbounded<IConnection>();
        private readonly Dictionary<uint, KcpSession> _sessions = new();
        private readonly CancellationTokenSource _cts = new();

        public KcpListener(IPAddress listenAddress, ushort listenPort)
        {
            _listenAddress = listenAddress;
            _listenPort = listenPort;
        }

        public void Start()
        {
            _ = Task.Run(ReceiveLoopAsync);
        }

        public async ValueTask<IConnection> AcceptAsync(CancellationToken cancellationToken = default)
        {
            return await _accept.Reader.ReadAsync(cancellationToken).ConfigureAwait(false);
        }

        private async Task ReceiveLoopAsync()
        {
            while (!_cts.IsCancellationRequested)
            {
                var channel = new RawTcpPacketChannel(_listenAddress, _listenPort);
                var result = await channel.ReceiveAsync(_cts.Token).ConfigureAwait(false);
                if (result.Payload.Length < 4)
                {
                    continue;
                }
                var conv = BinaryPrimitives.ReadUInt32LittleEndian(result.Payload.AsSpan(0, 4));
                if (!_sessions.TryGetValue(conv, out var session))
                {
                    var peer = result.Source;
                    var peerPort = result.SourcePort;
                    var localIp = ResolveLocalIPv4(peer);
                    var sessionChannel = new RawTcpPacketChannel(localIp, peer, peerPort);
                    session = new KcpSession(conv, sessionChannel);
                    session.Start();
                    _sessions[conv] = session;
                    _accept.Writer.TryWrite(new KcpConnection(session));
                }
                session.Input(result.Payload);
            }
        }

        public ValueTask DisposeAsync()
        {
            _cts.Cancel();
            return ValueTask.CompletedTask;
        }
    }

    private sealed class KcpConnection : IConnection
    {
        private readonly KcpSession _session;
        private int _streamClaimed;

        public KcpConnection(KcpSession session)
        {
            _session = session;
        }

        public ValueTask<IStream> OpenStreamAsync(CancellationToken cancellationToken = default)
        {
            return ValueTask.FromResult<IStream>(ClaimStream());
        }

        public ValueTask<IStream> AcceptStreamAsync(CancellationToken cancellationToken = default)
        {
            return ValueTask.FromResult<IStream>(ClaimStream());
        }

        private IStream ClaimStream()
        {
            if (Interlocked.Exchange(ref _streamClaimed, 1) == 1)
            {
                throw new InvalidOperationException("KCP transport currently supports a single stream per connection.");
            }
            return new KcpStream(_session);
        }

        public ValueTask DisposeAsync()
        {
            _session.Dispose();
            return ValueTask.CompletedTask;
        }
    }

    private sealed class KcpStream : IStream
    {
        private readonly KcpSession _session;
        private byte[]? _buffer;
        private int _offset;

        public KcpStream(KcpSession session)
        {
            _session = session;
        }

        public async ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken cancellationToken = default)
        {
            if (_buffer == null || _offset >= _buffer.Length)
            {
                _buffer = await _session.ReceiveAsync(cancellationToken).ConfigureAwait(false);
                _offset = 0;
            }

            var remaining = _buffer.Length - _offset;
            var toCopy = Math.Min(remaining, buffer.Length);
            _buffer.AsSpan(_offset, toCopy).CopyTo(buffer.Span);
            _offset += toCopy;
            return toCopy;
        }

        public async ValueTask WriteAsync(ReadOnlyMemory<byte> buffer, CancellationToken cancellationToken = default)
        {
            await _session.SendAsync(buffer, cancellationToken).ConfigureAwait(false);
        }

        public ValueTask DisposeAsync()
        {
            return ValueTask.CompletedTask;
        }
    }

    private sealed class KcpSession : IDisposable
    {
        private readonly KcpSharp.Kcp _kcp;
        private readonly RawTcpPacketChannel _channel;
        private readonly Channel<byte[]> _recv = Channel.CreateUnbounded<byte[]>();
        private readonly CancellationTokenSource _cts = new();
        private readonly object _sync = new();
        private Task? _updateLoop;
        private Task? _recvLoop;

        public KcpSession(uint conv, RawTcpPacketChannel channel)
        {
            _channel = channel;
            _kcp = new KcpSharp.Kcp(conv, Output);
            _kcp.NoDelay(1, 10, 2, 1);
            _kcp.WndSize(128, 128);
        }

        public void Start()
        {
            _recvLoop = Task.Run(ReceiveLoopAsync);
            _updateLoop = Task.Run(UpdateLoopAsync);
        }

        public void Input(byte[] data)
        {
            lock (_sync)
            {
                _kcp.Input(data);
            }
            Drain();
        }

        public async ValueTask SendAsync(ReadOnlyMemory<byte> data, CancellationToken cancellationToken)
        {
            var payload = new byte[data.Length + 2];
            BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0, 2), (ushort)data.Length);
            data.CopyTo(payload.AsMemory(2));
            lock (_sync)
            {
                _kcp.Send(payload);
            }
            await Task.Yield();
        }

        public async ValueTask<byte[]> ReceiveAsync(CancellationToken cancellationToken)
        {
            var packet = await _recv.Reader.ReadAsync(cancellationToken).ConfigureAwait(false);
            var length = BinaryPrimitives.ReadUInt16BigEndian(packet.AsSpan(0, 2));
            var payload = new byte[length];
            packet.AsSpan(2, length).CopyTo(payload);
            return payload;
        }

        private void Output(byte[] buffer, int size)
        {
            _channel.Send(buffer.AsSpan(0, size));
        }

        private async Task ReceiveLoopAsync()
        {
            while (!_cts.IsCancellationRequested)
            {
                var result = await _channel.ReceiveAsync(_cts.Token).ConfigureAwait(false);
                Input(result.Payload);
            }
        }

        private async Task UpdateLoopAsync()
        {
            while (!_cts.IsCancellationRequested)
            {
                lock (_sync)
                {
                    _kcp.Update(Environment.TickCount64);
                }
                await Task.Delay(10, _cts.Token).ConfigureAwait(false);
            }
        }

        private void Drain()
        {
            while (true)
            {
                int size;
                lock (_sync)
                {
                    size = _kcp.PeekSize();
                    if (size <= 0)
                    {
                        return;
                    }
                    var buffer = new byte[size];
                    var n = _kcp.Receive(buffer);
                    if (n > 0)
                    {
                        _recv.Writer.TryWrite(buffer);
                    }
                }
            }
        }

        public void Dispose()
        {
            _cts.Cancel();
        }
    }

    private static IPAddress ResolveLocalIPv4(IPAddress remote)
    {
        using var socket = new System.Net.Sockets.Socket(AddressFamily.InterNetwork, SocketType.Dgram, System.Net.Sockets.ProtocolType.Udp);
        socket.Connect(new IPEndPoint(remote, 9));
        return ((IPEndPoint)socket.LocalEndPoint!).Address;
    }

    private sealed class RawTcpPacketChannel : IDisposable
    {
        private readonly RawPacketSender _sender;
        private readonly RawPacketReceiver _receiver;
        private readonly TcpPacketState _state = new();
        private readonly IPAddress _source;
        private readonly IPAddress _destination;
        private readonly ushort _destinationPort;
        private readonly ushort _listenPort;

        public RawTcpPacketChannel(IPAddress source, ushort listenPort = 0)
        {
            _source = source;
            _destination = IPAddress.Any;
            _destinationPort = 0;
            _listenPort = listenPort;
            _sender = new RawPacketSender(source);
            _receiver = new RawPacketReceiver(source);
        }

        public RawTcpPacketChannel(IPAddress source, IPAddress destination, ushort destinationPort)
        {
            _source = source;
            _destination = destination;
            _destinationPort = destinationPort;
            _listenPort = 0;
            _sender = new RawPacketSender(source);
            _receiver = new RawPacketReceiver(source);
        }

        public void Send(ReadOnlySpan<byte> payload)
        {
            if (_destinationPort == 0 || IPAddress.Any.Equals(_destination) || IPAddress.Any.Equals(_source))
            {
                return;
            }
            _sender.Send(_source, _destination, 40000, _destinationPort, _state, payload);
        }

        public async ValueTask<(IPAddress Source, ushort SourcePort, byte[] Payload)> ReceiveAsync(CancellationToken cancellationToken)
        {
            var buffer = new byte[1500];
            while (!cancellationToken.IsCancellationRequested)
            {
                var count = _receiver.Receive(buffer);
                if (count <= 0)
                {
                    await Task.Delay(1, cancellationToken).ConfigureAwait(false);
                    continue;
                }
                if (count < 40)
                {
                    continue;
                }
                var ipHeaderLen = (buffer[0] & 0x0F) * 4;
                if (buffer[9] != 6 || count < ipHeaderLen + 20)
                {
                    continue;
                }
                var tcpOffset = ipHeaderLen;
                var dstPort = (ushort)((buffer[tcpOffset + 2] << 8) | buffer[tcpOffset + 3]);
                if (_listenPort != 0 && dstPort != _listenPort)
                {
                    continue;
                }
                var dataOffset = (buffer[tcpOffset + 12] >> 4) * 4;
                var payloadOffset = tcpOffset + dataOffset;
                if (payloadOffset > count)
                {
                    continue;
                }
                var srcIp = new IPAddress(buffer.AsSpan(12, 4));
                var srcPort = (ushort)((buffer[tcpOffset] << 8) | buffer[tcpOffset + 1]);
                if (srcPort == 0)
                {
                    continue;
                }
                var payload = buffer.AsSpan(payloadOffset, count - payloadOffset).ToArray();
                return (srcIp, srcPort, payload);
            }
            return (IPAddress.Any, 0, Array.Empty<byte>());
        }

        public void Dispose()
        {
            _sender.Dispose();
            _receiver.Dispose();
        }
    }
}
