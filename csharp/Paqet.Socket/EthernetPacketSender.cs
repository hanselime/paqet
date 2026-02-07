using System.Net;
using System.Net.NetworkInformation;
using PacketDotNet;
using SharpPcap;
using Paqet.Core;

namespace Paqet.Socket;

public sealed class EthernetPacketSender : IDisposable
{
    private readonly ICaptureDevice _device;
    private readonly PhysicalAddress _sourceMac;
    private readonly PhysicalAddress _gatewayMac;
    private readonly IPAddress _sourceAddress;

    public EthernetPacketSender(string deviceName, IPAddress sourceAddress, PhysicalAddress sourceMac, PhysicalAddress gatewayMac)
    {
        _device = CaptureDeviceList.Instance.FirstOrDefault(d => d.Name == deviceName)
                  ?? throw new InvalidOperationException($"Capture device not found: {deviceName}");
        _sourceAddress = sourceAddress;
        _sourceMac = sourceMac;
        _gatewayMac = gatewayMac;
        _device.Open(DeviceModes.Promiscuous, read_timeout: 1);
    }

    public void Send(IPAddress destination, ushort sourcePort, ushort destPort, TcpFlags flags, uint seq, uint ack, ReadOnlySpan<byte> payload)
    {
        var tcp = new TcpPacket(sourcePort, destPort)
        {
            SequenceNumber = seq,
            AcknowledgmentNumber = ack,
            Fin = flags.Fin,
            Syn = flags.Syn,
            Rst = flags.Rst,
            Psh = flags.Psh,
            Ack = flags.Ack,
            Urg = flags.Urg,
            EcnEcho = flags.Ece,
            Cwr = flags.Cwr,
            WindowSize = 65535
        };
        tcp.PayloadData = payload.ToArray();

        var ip = new IPv4Packet(_sourceAddress, destination)
        {
            TimeToLive = 64,
            Protocol = System.Net.Sockets.ProtocolType.Tcp
        };
        ip.PayloadPacket = tcp;

        var eth = new EthernetPacket(_sourceMac, _gatewayMac, EthernetType.IPv4)
        {
            PayloadPacket = ip
        };

        tcp.UpdateCalculatedValues();
        ip.UpdateCalculatedValues();
        eth.UpdateCalculatedValues();

        _device.SendPacket(eth);
    }

    public void Send(IPAddress destination, ushort sourcePort, ushort destPort, TcpPacketState state, ReadOnlySpan<byte> payload)
    {
        var (seq, ack, _, flags) = state.Next(payload.Length);
        Send(destination, sourcePort, destPort, flags, seq, ack, payload);
    }

    public void Dispose()
    {
        _device.Close();
    }
}
