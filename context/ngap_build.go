package context

import (
	"encoding/binary"
	"fmt"

	"bitbucket.org/free5gc-team/aper"
	"bitbucket.org/free5gc-team/ngap/ngapType"
)

func BuildPDUSessionResourceSetupRequestTransfer(ctx *SMContext) ([]byte, error) {

	var ANUPF = ctx.Tunnel.DataPathPool.GetDefaultPath().FirstDPNode
	var UpNode = ANUPF.UPF
	var teidOct = make([]byte, 4)
	binary.BigEndian.PutUint32(teidOct, ANUPF.UpLinkTunnel.TEID)

	resourceSetupRequestTransfer := ngapType.PDUSessionResourceSetupRequestTransfer{}

	// UL NG-U UP TNL Information
	ie := ngapType.PDUSessionResourceSetupRequestTransferIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDULNGUUPTNLInformation
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	if n3IP, err := UpNode.N3Interfaces[0].IP(ctx.SelectedPDUSessionType); err != nil {
		return nil, err
	} else {

		ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentULNGUUPTNLInformation,
			ULNGUUPTNLInformation: &ngapType.UPTransportLayerInformation{
				Present: ngapType.UPTransportLayerInformationPresentGTPTunnel,
				GTPTunnel: &ngapType.GTPTunnel{
					TransportLayerAddress: ngapType.TransportLayerAddress{
						Value: aper.BitString{
							Bytes:     n3IP,
							BitLength: uint64(len(n3IP) * 8),
						},
					},
					GTPTEID: ngapType.GTPTEID{Value: teidOct},
				},
			},
		}
	}

	resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)

	// PDU Session Type
	ie = ngapType.PDUSessionResourceSetupRequestTransferIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDPDUSessionType
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
		Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentPDUSessionType,
		PDUSessionType: &ngapType.PDUSessionType{
			Value: ngapType.PDUSessionTypePresentIpv4,
		},
	}
	resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)

	// QoS Flow Setup Request List
	// use Default 5qi, arp
	ie = ngapType.PDUSessionResourceSetupRequestTransferIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDQosFlowSetupRequestList
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
		Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentQosFlowSetupRequestList,
		QosFlowSetupRequestList: &ngapType.QosFlowSetupRequestList{
			List: []ngapType.QosFlowSetupRequestItem{
				{
					QosFlowIdentifier: ngapType.QosFlowIdentifier{
						Value: 0,
					},
					QosFlowLevelQosParameters: ngapType.QosFlowLevelQosParameters{
						QosCharacteristics: ngapType.QosCharacteristics{
							Present: ngapType.QosCharacteristicsPresentNonDynamic5QI,
							NonDynamic5QI: &ngapType.NonDynamic5QIDescriptor{
								FiveQI: ngapType.FiveQI{
									Value: 9,
								},
							},
						},
						AllocationAndRetentionPriority: ngapType.AllocationAndRetentionPriority{
							PriorityLevelARP: ngapType.PriorityLevelARP{
								Value: 15,
							},
							PreEmptionCapability: ngapType.PreEmptionCapability{
								Value: ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption,
							},
							PreEmptionVulnerability: ngapType.PreEmptionVulnerability{
								Value: ngapType.PreEmptionVulnerabilityPresentNotPreEmptable,
							},
						},
					},
				},
			},
		},
	}

	resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)

	if buf, err := aper.MarshalWithParams(resourceSetupRequestTransfer, "valueExt"); err != nil {
		return nil, fmt.Errorf("encode resourceSetupRequestTransfer failed: %s", err)
	} else {
		return buf, nil
	}
}

// TS 38.413 9.3.4.9
func BuildPathSwitchRequestAcknowledgeTransfer(ctx *SMContext) ([]byte, error) {
	var ANUPF = ctx.Tunnel.DataPathPool.GetDefaultPath().FirstDPNode
	var UpNode = ANUPF.UPF
	var teidOct = make([]byte, 4)
	binary.BigEndian.PutUint32(teidOct, ANUPF.UpLinkTunnel.TEID)

	pathSwitchRequestAcknowledgeTransfer := ngapType.PathSwitchRequestAcknowledgeTransfer{}

	// UL NG-U UP TNL Information(optional) TS 38.413 9.3.2.2
	pathSwitchRequestAcknowledgeTransfer.
		ULNGUUPTNLInformation = new(ngapType.UPTransportLayerInformation)

	ULNGUUPTNLInformation := pathSwitchRequestAcknowledgeTransfer.ULNGUUPTNLInformation
	ULNGUUPTNLInformation.Present = ngapType.UPTransportLayerInformationPresentGTPTunnel
	ULNGUUPTNLInformation.GTPTunnel = new(ngapType.GTPTunnel)

	gtpTunnel := pathSwitchRequestAcknowledgeTransfer.ULNGUUPTNLInformation.GTPTunnel
	gtpTunnel.GTPTEID.Value = teidOct
	gtpTunnel.TransportLayerAddress.Value = aper.BitString{
		Bytes:     UpNode.NodeID.NodeIdValue,
		BitLength: uint64(len(UpNode.NodeID.NodeIdValue) * 8),
	}

	// Security Indication(optional) TS 38.413 9.3.1.27
	pathSwitchRequestAcknowledgeTransfer.SecurityIndication = new(ngapType.SecurityIndication)
	securityIndication := pathSwitchRequestAcknowledgeTransfer.SecurityIndication
	// TODO: use real value
	securityIndication.IntegrityProtectionIndication.Value = ngapType.IntegrityProtectionIndicationPresentNotNeeded
	// TODO: use real value
	securityIndication.ConfidentialityProtectionIndication.Value =
		ngapType.ConfidentialityProtectionIndicationPresentNotNeeded

	integrityProtectionInd := securityIndication.IntegrityProtectionIndication.Value
	if integrityProtectionInd == ngapType.IntegrityProtectionIndicationPresentRequired ||
		integrityProtectionInd == ngapType.IntegrityProtectionIndicationPresentPreferred {
		securityIndication.MaximumIntegrityProtectedDataRateUL = new(ngapType.MaximumIntegrityProtectedDataRate)
		// TODO: use real value
		securityIndication.MaximumIntegrityProtectedDataRateUL.Value =
			ngapType.MaximumIntegrityProtectedDataRatePresentBitrate64kbs
	}

	if buf, err := aper.MarshalWithParams(pathSwitchRequestAcknowledgeTransfer, "valueExt"); err != nil {
		return nil, err
	} else {
		return buf, nil
	}
}

func BuildPathSwitchRequestUnsuccessfulTransfer(causePresent int, causeValue aper.Enumerated) (buf []byte, err error) {

	pathSwitchRequestUnsuccessfulTransfer := ngapType.PathSwitchRequestUnsuccessfulTransfer{}

	pathSwitchRequestUnsuccessfulTransfer.Cause.Present = causePresent
	cause := &pathSwitchRequestUnsuccessfulTransfer.Cause

	switch causePresent {
	case ngapType.CausePresentRadioNetwork:
		cause.RadioNetwork = new(ngapType.CauseRadioNetwork)
		cause.RadioNetwork.Value = causeValue
	case ngapType.CausePresentTransport:
		cause.Transport = new(ngapType.CauseTransport)
		cause.Transport.Value = causeValue
	case ngapType.CausePresentNas:
		cause.Nas = new(ngapType.CauseNas)
		cause.Nas.Value = causeValue
	case ngapType.CausePresentProtocol:
		cause.Protocol = new(ngapType.CauseProtocol)
		cause.Protocol.Value = causeValue
	case ngapType.CausePresentMisc:
		cause.Misc = new(ngapType.CauseMisc)
		cause.Misc.Value = causeValue
	}

	buf, err = aper.MarshalWithParams(pathSwitchRequestUnsuccessfulTransfer, "valueExt")
	if err != nil {
		return nil, err
	}
	return
}

func BuildPDUSessionResourceReleaseCommandTransfer(ctx *SMContext) (buf []byte, err error) {
	resourceReleaseCommandTransfer := ngapType.PDUSessionResourceReleaseCommandTransfer{
		Cause: ngapType.Cause{
			Present: ngapType.CausePresentNas,
			Nas: &ngapType.CauseNas{
				Value: ngapType.CauseNasPresentNormalRelease,
			},
		},
	}
	buf, err = aper.MarshalWithParams(resourceReleaseCommandTransfer, "valueExt")
	if err != nil {
		return nil, err
	}
	return
}

func BuildHandoverCommandTransfer(ctx *SMContext) (buf []byte, err error) {
	var ANUPF = ctx.Tunnel.DataPathPool.GetDefaultPath().FirstDPNode
	var UpNode = ANUPF.UPF
	var teidOct = make([]byte, 4)
	binary.BigEndian.PutUint32(teidOct, ANUPF.UpLinkTunnel.TEID)
	handoverCommandTransfer := ngapType.HandoverCommandTransfer{}

	handoverCommandTransfer.DLForwardingUPTNLInformation = new(ngapType.UPTransportLayerInformation)
	handoverCommandTransfer.DLForwardingUPTNLInformation.Present = ngapType.UPTransportLayerInformationPresentGTPTunnel
	handoverCommandTransfer.DLForwardingUPTNLInformation.GTPTunnel = new(ngapType.GTPTunnel)

	gtpTunnel := handoverCommandTransfer.DLForwardingUPTNLInformation.GTPTunnel
	gtpTunnel.GTPTEID.Value = teidOct
	gtpTunnel.TransportLayerAddress.Value = aper.BitString{
		Bytes:     UpNode.NodeID.NodeIdValue,
		BitLength: uint64(len(UpNode.NodeID.NodeIdValue) * 8),
	}

	buf, err = aper.MarshalWithParams(handoverCommandTransfer, "valueExt")
	if err != nil {
		return nil, err
	}
	return
}
