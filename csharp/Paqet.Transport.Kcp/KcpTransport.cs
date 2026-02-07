using System.Buffers.Binary;
using System.Net;
using System.Net.Sockets;
using System.Threading.Channels;
using KcpSharp;
using Paqet.Core;

namespace Paqet.Transport.Kcp;

public sealed class KcpTransport : ITransport
{
    public async ValueTask<IConnection> DialAsync(Address address, CancellationToken cancellationToken = default)
    {
        var udp = new UdpClient(0);
        var remote = new IPEndPoint(IPAddress.Parse(address.Host), address.Port);
        var conv = (uint)Random.Shared.Next(1, int.MaxValue);
        var session = new KcpSession(conv, udp, remote);
        session.Start();
        return await ValueTask.FromResult<IConnection>(new KcpConnection(session));
    }

    public async ValueTask<IListener> ListenAsync(Address address, CancellationToken cancellationToken = default)
    {
        var udp = new UdpClient(new IPEndPoint(IPAddress.Parse(address.Host), address.Port));
        var listener = new KcpListener(udp);
        listener.Start();
        return await ValueTask.FromResult<IListener>(listener);
    }

    private sealed class KcpListener : IListener
    {
        private readonly UdpClient _udp;
        private readonly Channel<IConnection> _accept = Channel.CreateUnbounded<IConnection>();
        private readonly Dictionary<uint, KcpSession> _sessions = new();
        private readonly CancellationTokenSource _cts = new();

        public KcpListener(UdpClient udp)
        {
            _udp = udp;
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
                var result = await _udp.ReceiveAsync(_cts.Token).ConfigureAwait(false);
                if (result.Buffer.Length < 4)
                {
                    continue;
                }
                var conv = BinaryPrimitives.ReadUInt32LittleEndian(result.Buffer.AsSpan(0, 4));
                if (!_sessions.TryGetValue(conv, out var session))
                {
                    session = new KcpSession(conv, _udp, result.RemoteEndPoint);
                    session.Start();
                    _sessions[conv] = session;
                    _accept.Writer.TryWrite(new KcpConnection(session));
                }
                session.Input(result.Buffer);
            }
        }

        public ValueTask DisposeAsync()
        {
            _cts.Cancel();
            _udp.Dispose();
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
        private readonly Kcp _kcp;
        private readonly UdpClient _udp;
        private readonly IPEndPoint _remote;
        private readonly Channel<byte[]> _recv = Channel.CreateUnbounded<byte[]>();
        private readonly CancellationTokenSource _cts = new();
        private readonly object _sync = new();
        private Task? _updateLoop;
        private Task? _recvLoop;

        public KcpSession(uint conv, UdpClient udp, IPEndPoint remote)
        {
            _udp = udp;
            _remote = remote;
            _kcp = new Kcp(conv, Output);
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
            _udp.Send(buffer, size, _remote);
        }

        private async Task ReceiveLoopAsync()
        {
            while (!_cts.IsCancellationRequested)
            {
                var result = await _udp.ReceiveAsync(_cts.Token).ConfigureAwait(false);
                if (!result.RemoteEndPoint.Equals(_remote))
                {
                    continue;
                }
                Input(result.Buffer);
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
}
