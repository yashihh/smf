package handler

import (
	"context"
	"fmt"

	"bitbucket.org/free5gc-team/openapi/models"
	"bitbucket.org/free5gc-team/pfcp"
	"bitbucket.org/free5gc-team/pfcp/pfcpType"
	"bitbucket.org/free5gc-team/pfcp/pfcpUdp"
	smf_context "bitbucket.org/free5gc-team/smf/internal/context"
	"bitbucket.org/free5gc-team/smf/internal/logger"
	pfcp_message "bitbucket.org/free5gc-team/smf/internal/pfcp/message"
)

func HandlePfcpHeartbeatRequest(msg *pfcpUdp.Message) {
	h := msg.PfcpMessage.Header
	pfcp_message.SendHeartbeatResponse(msg.RemoteAddr, h.SequenceNumber)
}

func HandlePfcpPfdManagementRequest(msg *pfcpUdp.Message) {
	logger.PfcpLog.Warnf("PFCP PFD Management Request handling is not implemented")
}

func HandlePfcpAssociationSetupRequest(msg *pfcpUdp.Message) {
	req := msg.PfcpMessage.Body.(pfcp.PFCPAssociationSetupRequest)

	nodeID := req.NodeID
	if nodeID == nil {
		logger.PfcpLog.Errorln("pfcp association needs NodeID")
		return
	}
	logger.PfcpLog.Infof("Handle PFCP Association Setup Request with NodeID[%s]",
		nodeID.ResolveNodeIdToIp().String())

	upf := smf_context.RetrieveUPFNodeByNodeID(*nodeID)
	if upf == nil {
		logger.PfcpLog.Errorf("can't find UPF[%s]", nodeID.ResolveNodeIdToIp().String())
		return
	}

	upf.UPIPInfo = *req.UserPlaneIPResourceInformation

	// Response with PFCP Association Setup Response
	cause := pfcpType.Cause{
		CauseValue: pfcpType.CauseRequestAccepted,
	}
	pfcp_message.SendPfcpAssociationSetupResponse(msg.RemoteAddr, cause)
}

func HandlePfcpAssociationUpdateRequest(msg *pfcpUdp.Message) {
	logger.PfcpLog.Warnf("PFCP Association Update Request handling is not implemented")
}

func HandlePfcpAssociationReleaseRequest(msg *pfcpUdp.Message) {
	pfcpMsg := msg.PfcpMessage.Body.(pfcp.PFCPAssociationReleaseRequest)

	var cause pfcpType.Cause
	upf := smf_context.RetrieveUPFNodeByNodeID(*pfcpMsg.NodeID)

	if upf != nil {
		smf_context.RemoveUPFNodeByNodeID(*pfcpMsg.NodeID)
		cause.CauseValue = pfcpType.CauseRequestAccepted
	} else {
		cause.CauseValue = pfcpType.CauseNoEstablishedPfcpAssociation
	}

	pfcp_message.SendPfcpAssociationReleaseResponse(msg.RemoteAddr, cause)
}

func HandlePfcpNodeReportRequest(msg *pfcpUdp.Message) {
	logger.PfcpLog.Warnf("PFCP Node Report Request handling is not implemented")
}

func HandlePfcpSessionSetDeletionRequest(msg *pfcpUdp.Message) {
	logger.PfcpLog.Warnf("PFCP Session Set Deletion Request handling is not implemented")
}

func HandlePfcpSessionSetDeletionResponse(msg *pfcpUdp.Message) {
	logger.PfcpLog.Warnf("PFCP Session Set Deletion Response handling is not implemented")
}

func HandlePfcpSessionReportRequest(msg *pfcpUdp.Message) {
	var cause pfcpType.Cause
	var remoteSEID uint64
	var nodeId pfcpType.NodeID

	req := msg.PfcpMessage.Body.(pfcp.PFCPSessionReportRequest)
	SEID := msg.PfcpMessage.Header.SEID
	smContext := smf_context.GetSMContextBySEID(SEID)
	seqFromUPF := msg.PfcpMessage.Header.SequenceNumber

	if smContext == nil {
		logger.PfcpLog.Errorf("PFCP Session SEID[%d] not found", SEID)
		cause.CauseValue = pfcpType.CauseSessionContextNotFound
		pfcp_message.SendPfcpSessionReportResponse(msg.RemoteAddr, cause, seqFromUPF, 0)
		return
	} else {
		nodeId = smContext.GetNodeIDByLocalSEID(SEID)
		nodeIDtoIP := nodeId.ResolveNodeIdToIp().String()
		if pfcpCtx := smContext.PFCPContext[nodeIDtoIP]; pfcpCtx == nil {
			logger.PfcpLog.Errorf("pfcpCtx [nodeId: %v, seid:%d] not found", nodeId, SEID)
			cause.CauseValue = pfcpType.CauseRequestRejected
			pfcp_message.SendPfcpSessionReportResponse(msg.RemoteAddr, cause, seqFromUPF, 0)
			return
		} else {
			remoteSEID = pfcpCtx.RemoteSEID
		}
	}

	smContext.SMLock.Lock()
	defer smContext.SMLock.Unlock()

	if smContext.UpCnxState == models.UpCnxState_DEACTIVATED {
		if req.ReportType.Dldr {
			downlinkDataReport := req.DownlinkDataReport

			if downlinkDataReport.DownlinkDataServiceInformation != nil {
				logger.PfcpLog.Warnf(
					"PFCP Session Report Request DownlinkDataServiceInformation handling is not implemented")
			}

			n1n2Request := models.N1N2MessageTransferRequest{}

			// TS 23.502 4.2.3.3 3a. Send Namf_Communication_N1N2MessageTransfer Request, SMF->AMF
			if n2SmBuf, err := smf_context.BuildPDUSessionResourceSetupRequestTransfer(smContext); err != nil {
				logger.PduSessLog.Errorln("Build PDUSessionResourceSetupRequestTransfer failed:", err)
			} else {
				n1n2Request.BinaryDataN2Information = n2SmBuf
			}

			n1n2Request.JsonData = &models.N1N2MessageTransferReqData{
				PduSessionId: smContext.PDUSessionID,
				// Temporarily assign SMF itself,
				// TODO: TS 23.502 4.2.3.3 5. Namf_Communication_N1N2TransferFailureNotification
				N1n2FailureTxfNotifURI: fmt.Sprintf("%s://%s:%d",
					smf_context.SMF_Self().URIScheme,
					smf_context.SMF_Self().RegisterIPv4,
					smf_context.SMF_Self().SBIPort),
				N2InfoContainer: &models.N2InfoContainer{
					N2InformationClass: models.N2InformationClass_SM,
					SmInfo: &models.N2SmInformation{
						PduSessionId: smContext.PDUSessionID,
						N2InfoContent: &models.N2InfoContent{
							NgapIeType: models.NgapIeType_PDU_RES_SETUP_REQ,
							NgapData: &models.RefToBinaryData{
								ContentId: "N2SmInformation",
							},
						},
						SNssai: smContext.SNssai,
					},
				},
			}

			rspData, _, err := smContext.CommunicationClient.
				N1N2MessageCollectionDocumentApi.
				N1N2MessageTransfer(context.Background(), smContext.Supi, n1n2Request)
			if err != nil {
				logger.PfcpLog.Warnf("Send N1N2Transfer failed: %s", err)
			}
			if rspData.Cause == models.N1N2MessageTransferCause_ATTEMPTING_TO_REACH_UE {
				logger.PfcpLog.Infof("Receive %v, AMF is able to page the UE", rspData.Cause)
			}
			if rspData.Cause == models.N1N2MessageTransferCause_UE_NOT_RESPONDING {
				logger.PfcpLog.Warnf("%v", rspData.Cause)
				// TODO: TS 23.502 4.2.3.3 3c. Failure indication
			}
		}
	}

	if req.ReportType.Usar && req.UsageReport != nil {
		HandleReports(req.UsageReport, nil, nil, smContext, nodeId)
	}

	// TS 23.502 4.2.3.3 2b. Send Data Notification Ack, SMF->UPF
	cause.CauseValue = pfcpType.CauseRequestAccepted
	pfcp_message.SendPfcpSessionReportResponse(msg.RemoteAddr, cause, seqFromUPF, remoteSEID)
}

func HandleReports(
	UsageReportReport []*pfcp.UsageReportPFCPSessionReportRequest,
	UsageReportModification []*pfcp.UsageReportPFCPSessionModificationResponse,
	UsageReportDeletion []*pfcp.UsageReportPFCPSessionDeletionResponse,
	smContext *smf_context.SMContext,
	nodeId pfcpType.NodeID,
) {
	var usageReport smf_context.UsageReport

	for _, report := range UsageReportReport {
		usageReport.UrrId = report.URRID.UrrIdValue
		usageReport.TotalVolume = report.VolumeMeasurement.TotalVolume
		usageReport.UplinkVolume = report.VolumeMeasurement.UplinkVolume
		usageReport.DownlinkVolume = report.VolumeMeasurement.DownlinkVolume
		usageReport.TotalPktNum = report.VolumeMeasurement.TotalPktNum
		usageReport.UplinkPktNum = report.VolumeMeasurement.UplinkPktNum
		usageReport.DownlinkPktNum = report.VolumeMeasurement.DownlinkPktNum

		smContext.UrrReports = append(smContext.UrrReports, usageReport)
	}
	for _, report := range UsageReportModification {
		usageReport.UrrId = report.URRID.UrrIdValue
		usageReport.TotalVolume = report.VolumeMeasurement.TotalVolume
		usageReport.UplinkVolume = report.VolumeMeasurement.UplinkVolume
		usageReport.DownlinkVolume = report.VolumeMeasurement.DownlinkVolume
		usageReport.TotalPktNum = report.VolumeMeasurement.TotalPktNum
		usageReport.UplinkPktNum = report.VolumeMeasurement.UplinkPktNum
		usageReport.DownlinkPktNum = report.VolumeMeasurement.DownlinkPktNum

		smContext.UrrReports = append(smContext.UrrReports, usageReport)
	}
	for _, report := range UsageReportDeletion {
		usageReport.UrrId = report.URRID.UrrIdValue
		usageReport.TotalVolume = report.VolumeMeasurement.TotalVolume
		usageReport.UplinkVolume = report.VolumeMeasurement.UplinkVolume
		usageReport.DownlinkVolume = report.VolumeMeasurement.DownlinkVolume
		usageReport.TotalPktNum = report.VolumeMeasurement.TotalPktNum
		usageReport.UplinkPktNum = report.VolumeMeasurement.UplinkPktNum
		usageReport.DownlinkPktNum = report.VolumeMeasurement.DownlinkPktNum

		smContext.UrrReports = append(smContext.UrrReports, usageReport)
	}
}
