using System.Diagnostics;
using Paqet.Core;

namespace Paqet.Socket;

public sealed class TcpPacketState
{
    private uint _seq;
    private uint _ack;
    private uint _ts;

    public TcpPacketState(uint initialSeq = 1, uint initialAck = 0)
    {
        _seq = initialSeq;
        _ack = initialAck;
        _ts = (uint)Stopwatch.GetTimestamp();
    }

    public (uint Seq, uint Ack, uint Timestamp) Next(TcpFlags flags, int payloadLength)
    {
        var seq = _seq;
        var ack = _ack;
        if (flags.Syn)
        {
            seq = _seq;
            _seq += 1;
        }
        else
        {
            _seq += (uint)payloadLength;
        }
        _ts += 1;
        return (seq, ack, _ts);
    }

    public void SetAck(uint ack)
    {
        _ack = ack;
    }
}
