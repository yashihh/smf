package producer

import (
	"bitbucket.org/free5gc-team/openapi/models"
	"bitbucket.org/free5gc-team/pfcp/pfcpType"
	smf_context "bitbucket.org/free5gc-team/smf/internal/context"
	"bitbucket.org/free5gc-team/smf/internal/logger"
	pfcp_message "bitbucket.org/free5gc-team/smf/internal/pfcp/message"
	"bitbucket.org/free5gc-team/smf/internal/util"
)

type PFCPState struct {
	upf     *smf_context.UPF
	pdrList []*smf_context.PDR
	farList []*smf_context.FAR
	qerList []*smf_context.QER
}

// SendPFCPRule send one datapath to UPF
func SendPFCPRule(smContext *smf_context.SMContext, dataPath *smf_context.DataPath) {
	logger.PduSessLog.Infoln("Send PFCP Rule")
	logger.PduSessLog.Infoln("DataPath: ", dataPath)
	for curDataPathNode := dataPath.FirstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {
		pdrList := make([]*smf_context.PDR, 0, 2)
		farList := make([]*smf_context.FAR, 0, 2)
		qerList := make([]*smf_context.QER, 0, 2)

		if curDataPathNode.UpLinkTunnel != nil && curDataPathNode.UpLinkTunnel.PDR != nil {
			pdrList = append(pdrList, curDataPathNode.UpLinkTunnel.PDR)
			farList = append(farList, curDataPathNode.UpLinkTunnel.PDR.FAR)
			if curDataPathNode.DownLinkTunnel.PDR.QER != nil {
				qerList = append(qerList, curDataPathNode.DownLinkTunnel.PDR.QER...)
			}
		}
		if curDataPathNode.DownLinkTunnel != nil && curDataPathNode.DownLinkTunnel.PDR != nil {
			pdrList = append(pdrList, curDataPathNode.DownLinkTunnel.PDR)
			farList = append(farList, curDataPathNode.DownLinkTunnel.PDR.FAR)
		}

		sessionContext, exist := smContext.PFCPContext[curDataPathNode.GetNodeIP()]
		if !exist || sessionContext.RemoteSEID == 0 {
			pfcp_message.SendPfcpSessionEstablishmentRequest(
				curDataPathNode.UPF.NodeID, curDataPathNode.UPF.Addr, smContext, pdrList, farList, nil, qerList)
		} else {
			pfcp_message.SendPfcpSessionModificationRequest(
				curDataPathNode.UPF.NodeID, curDataPathNode.UPF.Addr, smContext, pdrList, farList, nil, qerList)
		}
	}
}

// SendPFCPRules send all datapaths to UPFs
func SendPFCPRules(smContext *smf_context.SMContext) {
	pfcpPool := make(map[string]*PFCPState)

	for _, dataPath := range smContext.Tunnel.DataPathPool {
		if dataPath.Activated {
			for curDataPathNode := dataPath.FirstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {
				pdrList := make([]*smf_context.PDR, 0, 2)
				farList := make([]*smf_context.FAR, 0, 2)
				qerList := make([]*smf_context.QER, 0, 2)

				if curDataPathNode.UpLinkTunnel != nil && curDataPathNode.UpLinkTunnel.PDR != nil {
					pdrList = append(pdrList, curDataPathNode.UpLinkTunnel.PDR)
					farList = append(farList, curDataPathNode.UpLinkTunnel.PDR.FAR)
					if curDataPathNode.UpLinkTunnel.PDR.QER != nil {
						qerList = append(qerList, curDataPathNode.UpLinkTunnel.PDR.QER...)
					}
				}
				if curDataPathNode.DownLinkTunnel != nil && curDataPathNode.DownLinkTunnel.PDR != nil {
					pdrList = append(pdrList, curDataPathNode.DownLinkTunnel.PDR)
					farList = append(farList, curDataPathNode.DownLinkTunnel.PDR.FAR)
					// skip send QER because uplink and downlink shared one QER
				}

				pfcpState := pfcpPool[curDataPathNode.GetNodeIP()]
				if pfcpState == nil {
					pfcpPool[curDataPathNode.GetNodeIP()] = &PFCPState{
						upf:     curDataPathNode.UPF,
						pdrList: pdrList,
						farList: farList,
						qerList: qerList,
					}
				} else {
					pfcpState.pdrList = append(pfcpState.pdrList, pdrList...)
					pfcpState.farList = append(pfcpState.farList, farList...)
					pfcpState.qerList = append(pfcpState.qerList, qerList...)
				}
			}
		}
	}
	for ip, pfcp := range pfcpPool {
		sessionContext, exist := smContext.PFCPContext[ip]
		if !exist || sessionContext.RemoteSEID == 0 {
			pfcp_message.SendPfcpSessionEstablishmentRequest(
				pfcp.upf.NodeID, pfcp.upf.Addr, smContext, pfcp.pdrList, pfcp.farList, nil, pfcp.qerList)
		} else {
			pfcp_message.SendPfcpSessionModificationRequest(
				pfcp.upf.NodeID, pfcp.upf.Addr, smContext, pfcp.pdrList, pfcp.farList, nil, pfcp.qerList)
		}
	}
}

func createPccRuleDataPath(smContext *smf_context.SMContext,
	pccRule *smf_context.PCCRule,
	tcData *smf_context.TrafficControlData) {
	var targetDNAI string
	if tcData != nil && len(tcData.RouteToLocs) > 0 {
		targetDNAI = tcData.RouteToLocs[0].Dnai
	}
	upfSelectionParams := &smf_context.UPFSelectionParams{
		Dnn: smContext.Dnn,
		SNssai: &smf_context.SNssai{
			Sst: smContext.Snssai.Sst,
			Sd:  smContext.Snssai.Sd,
		},
		Dnai: targetDNAI,
	}
	createdUpPath := smf_context.GetUserPlaneInformation().GetDefaultUserPlanePathByDNN(upfSelectionParams)
	createdDataPath := smf_context.GenerateDataPath(createdUpPath, smContext)
	if createdDataPath != nil {
		createdDataPath.ActivateTunnelAndPDR(smContext, 255-uint32(pccRule.Precedence))
		smContext.Tunnel.AddDataPath(createdDataPath)
	}

	pccRule.Datapath = createdDataPath
}

func addQoSToDataPath(smContext *smf_context.SMContext, datapath *smf_context.DataPath, qos *models.QosData) {
	if qos == nil {
		return
	}
	for curDataPathNode := datapath.FirstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {
		if newQER, err := curDataPathNode.UPF.AddQER(); err != nil {
			logger.PduSessLog.Errorln("new QER failed")
			return
		} else {
			newQER.QFI.QFI = uint8(qos.Var5qi)
			newQER.GateStatus = &pfcpType.GateStatus{
				ULGate: pfcpType.GateOpen,
				DLGate: pfcpType.GateOpen,
			}
			newQER.MBR = &pfcpType.MBR{
				ULMBR: util.BitRateTokbps(qos.MaxbrUl),
				DLMBR: util.BitRateTokbps(qos.MaxbrDl),
			}
			newQER.GBR = &pfcpType.GBR{
				ULGBR: util.BitRateTokbps(qos.GbrUl),
				DLGBR: util.BitRateTokbps(qos.GbrDl),
			}

			if curDataPathNode.UpLinkTunnel != nil && curDataPathNode.UpLinkTunnel.PDR != nil {
				curDataPathNode.UpLinkTunnel.PDR.QER = append(curDataPathNode.UpLinkTunnel.PDR.QER, newQER)
			}
			if curDataPathNode.DownLinkTunnel != nil && curDataPathNode.DownLinkTunnel.PDR != nil {
				curDataPathNode.DownLinkTunnel.PDR.QER = append(curDataPathNode.DownLinkTunnel.PDR.QER, newQER)
			}
		}
	}
}

func removeDataPath(smContext *smf_context.SMContext, datapath *smf_context.DataPath) {
	for curDPNode := datapath.FirstDPNode; curDPNode != nil; curDPNode = curDPNode.Next() {
		if curDPNode.DownLinkTunnel != nil && curDPNode.DownLinkTunnel.PDR != nil {
			curDPNode.DownLinkTunnel.PDR.State = smf_context.RULE_REMOVE
			curDPNode.DownLinkTunnel.PDR.FAR.State = smf_context.RULE_REMOVE
		}
		if curDPNode.UpLinkTunnel != nil && curDPNode.UpLinkTunnel.PDR != nil {
			curDPNode.UpLinkTunnel.PDR.State = smf_context.RULE_REMOVE
			curDPNode.UpLinkTunnel.PDR.FAR.State = smf_context.RULE_REMOVE
		}
	}
}

// UpdateDataPathToUPF update the datapath of the UPF
func UpdateDataPathToUPF(smContext *smf_context.SMContext, oldDataPath, updateDataPath *smf_context.DataPath) {
	if oldDataPath == nil {
		SendPFCPRule(smContext, updateDataPath)
		return
	} else {
		removeDataPath(smContext, oldDataPath)
		SendPFCPRule(smContext, updateDataPath)
	}
}
