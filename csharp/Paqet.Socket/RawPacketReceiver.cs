using System.Net;
using System.Net.Sockets;

namespace Paqet.Socket;

public sealed class RawPacketReceiver : IDisposable
{
    private readonly Socket _socket;

    public RawPacketReceiver(IPAddress listenAddress)
    {
        _socket = new Socket(AddressFamily.InterNetwork, SocketType.Raw, System.Net.Sockets.ProtocolType.Tcp);
        _socket.Bind(new IPEndPoint(listenAddress, 0));
    }

    public int Receive(Span<byte> buffer)
    {
        return _socket.Receive(buffer);
    }

    public void Dispose()
    {
        _socket.Dispose();
    }
}
