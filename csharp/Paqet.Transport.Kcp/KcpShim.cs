using System.Collections.Concurrent;

namespace KcpSharp;

public sealed class Kcp
{
    private readonly Action<byte[], int> _output;
    private readonly ConcurrentQueue<byte[]> _recvQueue = new();

    public Kcp(uint conv, Action<byte[], int> output)
    {
        Conversation = conv;
        _output = output;
    }

    public uint Conversation { get; }

    public void NoDelay(int nodelay, int interval, int resend, int nc)
    {
    }

    public void WndSize(int sndwnd, int rcvwnd)
    {
    }

    public int Input(byte[] data)
    {
        _recvQueue.Enqueue(data);
        return 0;
    }

    public int Send(byte[] data)
    {
        _output(data, data.Length);
        return 0;
    }

    public int PeekSize()
    {
        return _recvQueue.TryPeek(out var data) ? data.Length : 0;
    }

    public int Receive(byte[] buffer)
    {
        if (!_recvQueue.TryDequeue(out var data))
        {
            return -1;
        }
        var length = Math.Min(buffer.Length, data.Length);
        Array.Copy(data, 0, buffer, 0, length);
        return length;
    }

    public void Update(long current)
    {
    }
}
