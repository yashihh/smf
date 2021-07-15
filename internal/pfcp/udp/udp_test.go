package udp_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"bitbucket.org/free5gc-team/pfcp"
	"bitbucket.org/free5gc-team/pfcp/pfcpType"
	"bitbucket.org/free5gc-team/pfcp/pfcpUdp"
	"bitbucket.org/free5gc-team/smf/internal/context"
	smf_pfcp "bitbucket.org/free5gc-team/smf/internal/pfcp"
	"bitbucket.org/free5gc-team/smf/internal/pfcp/udp"
)

const testPfcpClientPort = 12345

func TestRun(t *testing.T) {
	// Set SMF Node ID

	context.SMF_Self().CPNodeID = pfcpType.NodeID{
		NodeIdType: pfcpType.NodeIdTypeIpv4Address,
		IP:         net.ParseIP("127.0.0.1").To4(),
	}
	context.SMF_Self().ExtenalAddr = "127.0.0.1"
	context.SMF_Self().ListenAddr = "127.0.0.1"

	udp.Run(smf_pfcp.Dispatch)

	testPfcpReq := pfcp.Message{
		Header: pfcp.Header{
			Version:         1,
			MP:              0,
			S:               0,
			MessageType:     pfcp.PFCP_ASSOCIATION_SETUP_REQUEST,
			MessageLength:   9,
			SEID:            0,
			SequenceNumber:  1,
			MessagePriority: 0,
		},
		Body: pfcp.PFCPAssociationSetupRequest{
			NodeID: &pfcpType.NodeID{
				NodeIdType: 0,
				IP:         net.ParseIP("192.168.1.1").To4(),
			},
		},
	}

	srcAddr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: testPfcpClientPort,
	}
	dstAddr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: pfcpUdp.PFCP_PORT,
	}

	err := pfcpUdp.SendPfcpMessage(testPfcpReq, srcAddr, dstAddr)
	require.Nil(t, err)

	err = udp.Server.Close()
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)
}
