namespace Paqet.Core;

public sealed record TcpFlags(
    bool Fin,
    bool Syn,
    bool Rst,
    bool Psh,
    bool Ack,
    bool Urg,
    bool Ece,
    bool Cwr,
    bool Ns
);

public static class TcpFlagPresets
{
    public static readonly TcpFlags PshAck = new(false, false, false, true, true, false, false, false, false);
}
