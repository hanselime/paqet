using System.Buffers.Binary;
using System.Net;
using System.Net.Sockets;
using Paqet.Core;

namespace Paqet.Socket;

public sealed class RawPacketSender : IDisposable
{
    private readonly Socket _socket;

    public RawPacketSender(IPAddress sourceAddress)
    {
        _socket = new Socket(AddressFamily.InterNetwork, SocketType.Raw, ProtocolType.Tcp);
        _socket.SetSocketOption(SocketOptionLevel.IP, SocketOptionName.HeaderIncluded, true);
        _socket.Bind(new IPEndPoint(sourceAddress, 0));
    }

    public void Send(IPAddress source, IPAddress destination, ushort sourcePort, ushort destPort, TcpFlags flags, uint seq, uint ack, ReadOnlySpan<byte> payload)
    {
        var options = BuildTimestampOptions(0, 0);
        var tcpHeaderLength = 20 + options.Length;
        var buffer = new byte[20 + tcpHeaderLength + payload.Length];
        WriteIPv4Header(buffer.AsSpan(0, 20), source, destination, 20 + tcpHeaderLength + payload.Length);
        WriteTcpHeader(buffer.AsSpan(20, tcpHeaderLength), source, destination, sourcePort, destPort, flags, seq, ack, payload.Length, options);
        payload.CopyTo(buffer.AsSpan(20 + tcpHeaderLength));
        _socket.SendTo(buffer, new IPEndPoint(destination, destPort));
    }

    public void Send(IPAddress source, IPAddress destination, ushort sourcePort, ushort destPort, TcpPacketState state, ReadOnlySpan<byte> payload)
    {
        var flags = TcpFlagPresets.PshAck;
        var (seq, ack, ts) = state.Next(flags, payload.Length);
        var options = BuildTimestampOptions(ts, ts);
        var tcpHeaderLength = 20 + options.Length;
        var buffer = new byte[20 + tcpHeaderLength + payload.Length];
        WriteIPv4Header(buffer.AsSpan(0, 20), source, destination, 20 + tcpHeaderLength + payload.Length);
        WriteTcpHeader(buffer.AsSpan(20, tcpHeaderLength), source, destination, sourcePort, destPort, flags, seq, ack, payload.Length, options);
        payload.CopyTo(buffer.AsSpan(20 + tcpHeaderLength));
        _socket.SendTo(buffer, new IPEndPoint(destination, destPort));
    }

    private static void WriteIPv4Header(Span<byte> header, IPAddress source, IPAddress destination, int totalLength)
    {
        header[0] = 0x45;
        header[1] = 0;
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(2, 2), (ushort)totalLength);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(4, 2), 0);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(6, 2), 0x4000);
        header[8] = 64;
        header[9] = 6;
        header[10] = 0;
        header[11] = 0;
        source.GetAddressBytes().CopyTo(header.Slice(12, 4));
        destination.GetAddressBytes().CopyTo(header.Slice(16, 4));
        var checksum = ComputeChecksum(header);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(10, 2), checksum);
    }

    private static void WriteTcpHeader(Span<byte> header, IPAddress source, IPAddress destination, ushort sourcePort, ushort destPort, TcpFlags flags, uint seq, uint ack, int payloadLength, ReadOnlySpan<byte> options)
    {
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(0, 2), sourcePort);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(2, 2), destPort);
        BinaryPrimitives.WriteUInt32BigEndian(header.Slice(4, 4), seq);
        BinaryPrimitives.WriteUInt32BigEndian(header.Slice(8, 4), ack);
        header[12] = (byte)((header.Length / 4) << 4);
        header[13] = BuildFlags(flags);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(14, 2), 65535);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(16, 2), 0);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(18, 2), 0);
        if (!options.IsEmpty)
        {
            options.CopyTo(header.Slice(20));
        }

        var pseudo = new byte[12 + header.Length + payloadLength];
        source.GetAddressBytes().CopyTo(pseudo.AsSpan(0, 4));
        destination.GetAddressBytes().CopyTo(pseudo.AsSpan(4, 4));
        pseudo[8] = 0;
        pseudo[9] = 6;
        BinaryPrimitives.WriteUInt16BigEndian(pseudo.AsSpan(10, 2), (ushort)(header.Length + payloadLength));
        header.CopyTo(pseudo.AsSpan(12, header.Length));
        var checksum = ComputeChecksum(pseudo);
        BinaryPrimitives.WriteUInt16BigEndian(header.Slice(16, 2), checksum);
    }

    private static byte[] BuildTimestampOptions(uint tsVal, uint tsEcr)
    {
        var options = new byte[12];
        options[0] = 1;
        options[1] = 1;
        options[2] = 8;
        options[3] = 10;
        BinaryPrimitives.WriteUInt32BigEndian(options.AsSpan(4, 4), tsVal);
        BinaryPrimitives.WriteUInt32BigEndian(options.AsSpan(8, 4), tsEcr);
        return options;
    }

    private static byte BuildFlags(TcpFlags flags)
    {
        byte value = 0;
        if (flags.Fin) value |= 0x01;
        if (flags.Syn) value |= 0x02;
        if (flags.Rst) value |= 0x04;
        if (flags.Psh) value |= 0x08;
        if (flags.Ack) value |= 0x10;
        if (flags.Urg) value |= 0x20;
        if (flags.Ece) value |= 0x40;
        if (flags.Cwr) value |= 0x80;
        return value;
    }

    private static ushort ComputeChecksum(ReadOnlySpan<byte> data)
    {
        uint sum = 0;
        var i = 0;
        while (i + 1 < data.Length)
        {
            sum += (uint)((data[i] << 8) | data[i + 1]);
            i += 2;
        }
        if (i < data.Length)
        {
            sum += (uint)(data[i] << 8);
        }
        while ((sum >> 16) != 0)
        {
            sum = (sum & 0xFFFF) + (sum >> 16);
        }
        return (ushort)~sum;
    }

    public void Dispose()
    {
        _socket.Dispose();
    }
}
