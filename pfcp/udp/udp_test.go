package udp_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"free5gc/lib/pfcp"
	"free5gc/lib/pfcp/pfcpType"
	"free5gc/lib/pfcp/pfcpUdp"
	"free5gc/src/smf/handler"
	"free5gc/src/smf/pfcp"
	"free5gc/src/smf/pfcp/udp"
)

const testPfcpClientPort = 12345

func TestRun(t *testing.T) {
	// Set SMF Node ID
	udp.ServerNodeId = pfcpType.NodeID{
		NodeIdType:  pfcpType.NodeIdTypeIpv4Address,
		NodeIdValue: net.ParseIP("127.0.0.1").To4(),
	}

	go handler.Handle()
	udp.Run(pfcp.Dispatch)

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
				NodeIdType:  0,
				NodeIdValue: net.ParseIP("192.168.1.1").To4(),
			},
		},
	}

	srcAddr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: testPfcpClientPort,
	}
	dstAddr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: pfcpUdp.PfcpUdpDestinationPort,
	}

	err := pfcpUdp.SendPfcpMessage(testPfcpReq, srcAddr, dstAddr)
	assert.Nil(t, err)

	time.Sleep(300 * time.Millisecond)
}
