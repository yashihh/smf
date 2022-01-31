package producer

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/antihax/optional"

	"bitbucket.org/free5gc-team/nas"
	"bitbucket.org/free5gc-team/nas/nasMessage"
	"bitbucket.org/free5gc-team/openapi"
	"bitbucket.org/free5gc-team/openapi/Namf_Communication"
	"bitbucket.org/free5gc-team/openapi/Nsmf_PDUSession"
	"bitbucket.org/free5gc-team/openapi/Nudm_SubscriberDataManagement"
	"bitbucket.org/free5gc-team/openapi/models"
	"bitbucket.org/free5gc-team/pfcp/pfcpType"
	smf_context "bitbucket.org/free5gc-team/smf/internal/context"
	"bitbucket.org/free5gc-team/smf/internal/logger"
	pfcp_message "bitbucket.org/free5gc-team/smf/internal/pfcp/message"
	"bitbucket.org/free5gc-team/smf/internal/sbi/consumer"
	"bitbucket.org/free5gc-team/smf/pkg/factory"
	"bitbucket.org/free5gc-team/util/httpwrapper"
)

func HandlePDUSessionSMContextCreate(request models.PostSmContextsRequest) *httpwrapper.Response {
	// GSM State
	// PDU Session Establishment Accept/Reject
	var response models.PostSmContextsResponse
	response.JsonData = new(models.SmContextCreatedData)
	logger.PduSessLog.Infoln("In HandlePDUSessionSMContextCreate")

	// Check has PDU Session Establishment Request
	m := nas.NewMessage()
	if err := m.GsmMessageDecode(&request.BinaryDataN1SmMessage); err != nil ||
		m.GsmHeader.GetMessageType() != nas.MsgTypePDUSessionEstablishmentRequest {
		logger.PduSessLog.Warnln("GsmMessageDecode Error: ", err)
		httpResponse := &httpwrapper.Response{
			Header: nil,
			Status: http.StatusForbidden,
			Body: models.PostSmContextsErrorResponse{
				JsonData: &models.SmContextCreateError{
					Error: &Nsmf_PDUSession.N1SmError,
				},
			},
		}
		return httpResponse
	}

	createData := request.JsonData
	// Check duplicate SM Context
	if dup_smCtx := smf_context.GetSMContextById(createData.Supi, createData.PduSessionId); dup_smCtx != nil {
		HandlePDUSessionSMContextLocalRelease(dup_smCtx, createData)
	}

	smContext := smf_context.NewSMContext(createData.Supi, createData.PduSessionId)
	smContext.SetState(smf_context.ActivePending)
	smContext.SetCreateData(createData)
	smContext.SmStatusNotifyUri = createData.SmContextStatusUri

	// TODO: This lock should be unlock after PFPF Establishment,
	//       but this is workaround we must handle the pfcp timeout case to prevent SM lock allways lock
	smContext.SMLock.Lock()

	// DNN Information from config
	smContext.DNNInfo = smf_context.RetrieveDnnInformation(smContext.Snssai, smContext.Dnn)
	if smContext.DNNInfo == nil {
		logger.PduSessLog.Errorf("S-NSSAI[sst: %d, sd: %s] DNN[%s] not matched DNN Config",
			smContext.Snssai.Sst, smContext.Snssai.Sd, smContext.Dnn)
	}
	smContext.Log.Debugf("S-NSSAI[sst: %d, sd: %s] DNN[%s]",
		smContext.Snssai.Sst, smContext.Snssai.Sd, smContext.Dnn)

	// Query UDM
	if problemDetails, err := consumer.SendNFDiscoveryUDM(); err != nil {
		smContext.Log.Warnf("Send NF Discovery Serving UDM Error[%v]", err)
	} else if problemDetails != nil {
		smContext.Log.Warnf("Send NF Discovery Serving UDM Problem[%+v]", problemDetails)
	} else {
		smContext.Log.Infoln("Send NF Discovery Serving UDM Successfully")
	}

	smPlmnID := createData.Guami.PlmnId

	smDataParams := &Nudm_SubscriberDataManagement.GetSmDataParamOpts{
		Dnn:         optional.NewString(createData.Dnn),
		PlmnId:      optional.NewInterface(openapi.MarshToJsonString(smPlmnID)),
		SingleNssai: optional.NewInterface(openapi.MarshToJsonString(smContext.Snssai)),
	}

	SubscriberDataManagementClient := smf_context.SMF_Self().SubscriberDataManagementClient

	if sessSubData, rsp, err := SubscriberDataManagementClient.
		SessionManagementSubscriptionDataRetrievalApi.
		GetSmData(context.Background(), smContext.Supi, smDataParams); err != nil {
		smContext.Log.Errorln("Get SessionManagementSubscriptionData error:", err)
	} else {
		defer func() {
			if rspCloseErr := rsp.Body.Close(); rspCloseErr != nil {
				smContext.Log.Errorf("GetSmData response body cannot close: %+v", rspCloseErr)
			}
		}()
		if len(sessSubData) > 0 {
			smContext.DnnConfiguration = sessSubData[0].DnnConfigurations[smContext.Dnn]
			// UP Security info present in session management subscription data
			if smContext.DnnConfiguration.UpSecurity != nil {
				smContext.UpSecurity = smContext.DnnConfiguration.UpSecurity
			}
		} else {
			smContext.Log.Errorln("SessionManagementSubscriptionData from UDM is nil")
		}
	}

	// IP Allocation
	upfSelectionParams := &smf_context.UPFSelectionParams{
		Dnn: smContext.Dnn,
		SNssai: &smf_context.SNssai{
			Sst: smContext.Snssai.Sst,
			Sd:  smContext.Snssai.Sd,
		},
	}

	if len(smContext.DnnConfiguration.StaticIpAddress) > 0 {
		staticIPConfig := smContext.DnnConfiguration.StaticIpAddress[0]
		if staticIPConfig.Ipv4Addr != "" {
			upfSelectionParams.PDUAddress = net.ParseIP(staticIPConfig.Ipv4Addr).To4()
			smContext.UseStaticIP = true
		} else {
			smContext.UseStaticIP = false
		}
	} else {
		smContext.UseStaticIP = false
	}

	var selectedUPF *smf_context.UPNode
	var ip net.IP
	selectedUPFName := ""
	if smf_context.SMF_Self().ULCLSupport && smf_context.CheckUEHasPreConfig(createData.Supi) {
		groupName := smf_context.GetULCLGroupNameFromSUPI(createData.Supi)
		defaultPathPool := smf_context.GetUEDefaultPathPool(groupName)
		if defaultPathPool != nil {
			selectedUPFName, ip = defaultPathPool.SelectUPFAndAllocUEIPForULCL(
				smf_context.GetUserPlaneInformation(), upfSelectionParams)
			selectedUPF = smf_context.GetUserPlaneInformation().UPFs[selectedUPFName]
		}
	} else {
		selectedUPF, ip = smf_context.GetUserPlaneInformation().SelectUPFAndAllocUEIP(upfSelectionParams)
		smContext.PDUAddress = ip
		smContext.Log.Infof("Allocated PDUAdress[%s]", smContext.PDUAddress.String())
	}

	var httpResponse *httpwrapper.Response
	if ip == nil && (selectedUPF == nil || selectedUPFName == "") {
		smContext.Log.Error("fail allocate PDU address")

		smContext.SetState(smf_context.InActive)
		smContext.Log.Warnf("Data Path not found\n")
		smContext.Log.Warnln("Selection Parameter: ", upfSelectionParams.String())

		return makeErrorResponse(smContext, nasMessage.Cause5GSMInsufficientResourcesForSpecificSliceAndDNN,
			&Nsmf_PDUSession.InsufficientResourceSliceDnn)
	}
	smContext.PDUAddress = ip
	smContext.SelectedUPF = selectedUPF

	establishmentRequest := m.PDUSessionEstablishmentRequest
	if err := HandlePDUSessionEstablishmentRequest(smContext, establishmentRequest); err != nil {
		smContext.Log.Errorf("PDU Session Establishment fail by %s", err)
		gsmError := &GSMError{}
		if errors.As(err, &gsmError) {
			httpResponse = makeErrorResponse(smContext, gsmError.GSMCause, &Nsmf_PDUSession.N1SmError)
		} else {
			httpResponse = makeErrorResponse(smContext, nasMessage.Cause5GSMRequestRejectedUnspecified,
				&Nsmf_PDUSession.N1SmError)
		}
		return httpResponse
	}

	if err := smContext.PCFSelection(); err != nil {
		smContext.Log.Errorln("pcf selection error:", err)
	}

	var smPolicyDecision *models.SmPolicyDecision
	if smPolicyID, smPolicyDecisionRsp, err := consumer.SendSMPolicyAssociationCreate(smContext); err != nil {
		openapiError := err.(openapi.GenericOpenAPIError)
		problemDetails := openapiError.Model().(models.ProblemDetails)
		smContext.Log.Errorln("setup sm policy association failed:", err, problemDetails)
		smContext.SetState(smf_context.InActive)

		if problemDetails.Cause == "USER_UNKNOWN" {
			return makeErrorResponse(smContext, nasMessage.Cause5GSMRequestRejectedUnspecified,
				&Nsmf_PDUSession.SubscriptionDenied)
		} else {
			return makeErrorResponse(smContext, nasMessage.Cause5GSMNetworkFailure, &Nsmf_PDUSession.NetworkFailure)
		}
	} else {
		smPolicyDecision = smPolicyDecisionRsp
		smContext.SMPolicyID = smPolicyID
	}

	// dataPath selection
	smContext.Tunnel = smf_context.NewUPTunnel()
	if err := ApplySmPolicyFromDecision(smContext, smPolicyDecision); err != nil {
		smContext.Log.Errorf("apply sm policy decision error: %+v", err)
	}
	var defaultPath *smf_context.DataPath

	if smf_context.SMF_Self().ULCLSupport && smf_context.CheckUEHasPreConfig(createData.Supi) {
		smContext.Log.Infof("Has pre-config route")
		uePreConfigPaths := smf_context.GetUEPreConfigPaths(createData.Supi, selectedUPFName)
		smContext.Tunnel.DataPathPool = uePreConfigPaths.DataPathPool
		smContext.Tunnel.PathIDGenerator = uePreConfigPaths.PathIDGenerator
		defaultPath = smContext.Tunnel.DataPathPool.GetDefaultPath()
		defaultPath.ActivateTunnelAndPDR(smContext, 255)
		smContext.BPManager = smf_context.NewBPManager(createData.Supi)
	} else {
		// UE has no pre-config path.
		// Use default route
		smContext.Log.Infof("Has no pre-config route")
		defaultUPPath := smf_context.GetUserPlaneInformation().GetDefaultUserPlanePathByDNNAndUPF(
			upfSelectionParams, smContext.SelectedUPF)
		defaultPath = smf_context.GenerateDataPath(defaultUPPath, smContext)
		if defaultPath != nil {
			defaultPath.IsDefaultPath = true
			smContext.Tunnel.AddDataPath(defaultPath)
			defaultPath.ActivateTunnelAndPDR(smContext, 255)
		}
	}

	if defaultPath == nil {
		smContext.SetState(smf_context.InActive)
		smContext.Log.Warnf("Data Path not found\n")
		smContext.Log.Warnln("Selection Parameter: ", upfSelectionParams.String())

		return makeErrorResponse(smContext, nasMessage.Cause5GSMInsufficientResourcesForSpecificSliceAndDNN,
			&Nsmf_PDUSession.InsufficientResourceSliceDnn)
	}

	if problemDetails, err := consumer.SendNFDiscoveryServingAMF(smContext); err != nil {
		smContext.Log.Warnf("Send NF Discovery Serving AMF Error[%v]", err)
	} else if problemDetails != nil {
		smContext.Log.Warnf("Send NF Discovery Serving AMF Problem[%+v]", problemDetails)
	} else {
		smContext.Log.Traceln("Send NF Discovery Serving AMF successfully")
	}

	for _, service := range *smContext.AMFProfile.NfServices {
		if service.ServiceName == models.ServiceName_NAMF_COMM {
			communicationConf := Namf_Communication.NewConfiguration()
			communicationConf.SetBasePath(service.ApiPrefix)
			smContext.CommunicationClient = Namf_Communication.NewAPIClient(communicationConf)
		}
	}

	// Add sm lock timer workaround, it Should be remove after PFCP trasaction function complete
	smContext.SMLockTimer = time.AfterFunc(4*time.Second, func() {
		smContext.SMLock.Unlock()
	})
	SendPFCPRules(smContext)

	response.JsonData = smContext.BuildCreatedData()
	httpResponse = &httpwrapper.Response{
		Header: http.Header{
			"Location": {smContext.Ref},
		},
		Status: http.StatusCreated,
		Body:   response,
	}

	return httpResponse
	// TODO: UECM registration
}

func HandlePDUSessionSMContextUpdate(smContextRef string, body models.UpdateSmContextRequest) *httpwrapper.Response {
	// GSM State
	// PDU Session Modification Reject(Cause Value == 43 || Cause Value != 43)/Complete
	// PDU Session Release Command/Complete
	logger.PduSessLog.Infoln("In HandlePDUSessionSMContextUpdate")
	smContext := smf_context.GetSMContextByRef(smContextRef)

	if smContext == nil {
		logger.PduSessLog.Warnf("SMContext[%s] is not found", smContextRef)

		httpResponse := &httpwrapper.Response{
			Header: nil,
			Status: http.StatusNotFound,
			Body: models.UpdateSmContextErrorResponse{
				JsonData: &models.SmContextUpdateError{
					UpCnxState: models.UpCnxState_DEACTIVATED,
					Error: &models.ProblemDetails{
						Type:   "Resource Not Found",
						Title:  "SMContext Ref is not found",
						Status: http.StatusNotFound,
					},
				},
			},
		}
		return httpResponse
	}

	var httpResponse *httpwrapper.Response
	smContext.SMLock.Lock()
	defer smContext.SMLock.Unlock()

	var sendPFCPDelete, sendPFCPModification bool
	var response models.UpdateSmContextResponse
	response.JsonData = new(models.SmContextUpdatedData)

	smContextUpdateData := body.JsonData

	if body.BinaryDataN1SmMessage != nil {
		m := nas.NewMessage()
		err := m.GsmMessageDecode(&body.BinaryDataN1SmMessage)
		smContext.Log.Tracef("N1 Message: %s", hex.EncodeToString(body.BinaryDataN1SmMessage))
		if err != nil {
			smContext.Log.Errorf("N1 Message parse failed: %v", err)
			httpResponse = &httpwrapper.Response{
				Status: http.StatusForbidden,
				Body: models.UpdateSmContextErrorResponse{
					JsonData: &models.SmContextUpdateError{
						Error: &Nsmf_PDUSession.N1SmError,
					},
				}, // Depends on the reason why N4 fail
			}
			return httpResponse
		}

		switch m.GsmHeader.GetMessageType() {
		case nas.MsgTypePDUSessionReleaseRequest:
			smContext.CheckState(smf_context.Active)
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture

			HandlePDUSessionReleaseRequest(smContext, m.PDUSessionReleaseRequest)
			if smContext.SelectedUPF != nil {
				smContext.Log.Infof("Release IP[%s]", smContext.PDUAddress.String())
				smf_context.
					GetUserPlaneInformation().
					ReleaseUEIP(smContext.SelectedUPF, smContext.PDUAddress, smContext.UseStaticIP)
				smContext.SelectedUPF = nil
			}

			// remove SM Policy Association
			if smContext.SMPolicyID != "" {
				if err := consumer.SendSMPolicyAssociationTermination(smContext); err != nil {
					smContext.Log.Errorf("SM Policy Termination failed: %s", err)
				} else {
					smContext.SMPolicyID = ""
				}
			}

			if buf, err := smf_context.BuildGSMPDUSessionReleaseCommand(
				smContext, nasMessage.Cause5GSMRegularDeactivation); err != nil {
				smContext.Log.Errorf("Build GSM PDUSessionReleaseCommand failed: %+v", err)
			} else {
				response.BinaryDataN1SmMessage = buf
				sendGSMPDUSessionReleaseCommand(smContext, buf)
			}

			response.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionReleaseCommand"}

			response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "PDUResourceReleaseCommand"}
			response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_REL_CMD

			if buf, err := smf_context.BuildPDUSessionResourceReleaseCommandTransfer(smContext); err != nil {
				smContext.Log.Errorf("Build PDUSessionResourceReleaseCommandTransfer failed: %+v", err)
			} else {
				response.BinaryDataN2SmInformation = buf
			}

			smContext.SetState(smf_context.PFCPModification)

			releaseTunnel(smContext)

			sendPFCPDelete = true
		case nas.MsgTypePDUSessionReleaseComplete:
			smContext.CheckState(smf_context.InActivePending)
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture

			// Send Release Notify to AMF
			smContext.Log.Infoln("[SMF] Send Update SmContext Response")
			smContext.SetState(smf_context.InActive)
			response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED
			smContext.StopT3592()

			// If CN tunnel resource is released, should
			if smContext.Tunnel.ANInformation.IPAddress == nil {
				// Use go routine to send Notification to prevent blocking the handling process
				go sendSMContextStatusNotification(smContext)
				smf_context.RemoveSMContext(smContext.Ref)
			}
		case nas.MsgTypePDUSessionModificationRequest:
			if rsp, err := HandlePDUSessionModificationRequest(smContext, m.PDUSessionModificationRequest); err != nil {
				if buf, err := smf_context.BuildGSMPDUSessionModificationReject(smContext); err != nil {
					smContext.Log.Errorf("build GSM PDUSessionModificationReject failed: %+v", err)
				} else {
					response.BinaryDataN1SmMessage = buf
				}
			} else {
				if buf, err := rsp.PlainNasEncode(); err != nil {
					smContext.Log.Errorf("build GSM PDUSessionModificationCommand failed: %+v", err)
				} else {
					response.BinaryDataN1SmMessage = buf
					sendGSMPDUSessionModificationCommand(smContext, buf)
				}
			}

			response.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionModificationReject"}
			return &httpwrapper.Response{
				Status: http.StatusOK,
				Body:   response,
			}
		case nas.MsgTypePDUSessionModificationComplete:
			smContext.StopT3591()
		case nas.MsgTypePDUSessionModificationReject:
			smContext.StopT3591()
		}
	}

	tunnel := smContext.Tunnel
	pdrList := []*smf_context.PDR{}
	farList := []*smf_context.FAR{}
	barList := []*smf_context.BAR{}
	qerList := []*smf_context.QER{}

	switch smContextUpdateData.UpCnxState {
	case models.UpCnxState_ACTIVATING:
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.SetState(smf_context.ModificationPending)
		response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "PDUSessionResourceSetupRequestTransfer"}
		response.JsonData.UpCnxState = models.UpCnxState_ACTIVATING
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ

		n2Buf, err := smf_context.BuildPDUSessionResourceSetupRequestTransfer(smContext)
		if err != nil {
			smContext.Log.Errorf("Build PDUSession Resource Setup Request Transfer Error(%s)", err.Error())
		}
		smContext.UpCnxState = models.UpCnxState_ACTIVATING
		response.BinaryDataN2SmInformation = n2Buf
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ
	case models.UpCnxState_DEACTIVATED:
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		// If the PDU session has been released, skip sending PFCP Session Modification Request
		if smContext.CheckState(smf_context.InActivePending) {
			logger.CtxLog.Infof("Skip sending PFCP Session Modification Request of PDUSessionID:%d of SUPI:%s",
				smContext.PDUSessionID, smContext.Supi)
			response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED
			return &httpwrapper.Response{
				Status: http.StatusOK,
				Body:   response,
			}
		}
		smContext.SetState(smf_context.ModificationPending)
		response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED
		smContext.UpCnxState = body.JsonData.UpCnxState
		smContext.UeLocation = body.JsonData.UeLocation

		// Set FAR and An, N3 Release Info
		// TODO: Deactivate all datapath in ANUPF
		farList = []*smf_context.FAR{}
		smContext.PendingUPF = make(smf_context.PendingUPF)
		for _, dataPath := range smContext.Tunnel.DataPathPool {
			ANUPF := dataPath.FirstDPNode
			DLPDR := ANUPF.DownLinkTunnel.PDR
			if DLPDR == nil {
				smContext.Log.Warnf("Access network resource is released")
			} else {
				DLPDR.FAR.State = smf_context.RULE_UPDATE
				DLPDR.FAR.ApplyAction.Forw = false
				DLPDR.FAR.ApplyAction.Buff = true
				DLPDR.FAR.ApplyAction.Nocp = true
				smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				farList = append(farList, DLPDR.FAR)
				sendPFCPModification = true
				smContext.SetState(smf_context.PFCPModification)
			}
		}
	}

	switch smContextUpdateData.N2SmInfoType {
	case models.N2SmInfoType_PDU_RES_SETUP_RSP:
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.SetState(smf_context.ModificationPending)
		pdrList = []*smf_context.PDR{}
		farList = []*smf_context.FAR{}

		smContext.PendingUPF = make(smf_context.PendingUPF)
		for _, dataPath := range tunnel.DataPathPool {
			if dataPath.Activated {
				ANUPF := dataPath.FirstDPNode
				DLPDR := ANUPF.DownLinkTunnel.PDR

				DLPDR.FAR.ApplyAction = pfcpType.ApplyAction{Buff: false, Drop: false, Dupl: false, Forw: true, Nocp: false}
				DLPDR.FAR.ForwardingParameters = &smf_context.ForwardingParameters{
					DestinationInterface: pfcpType.DestinationInterface{
						InterfaceValue: pfcpType.DestinationInterfaceAccess,
					},
					NetworkInstance: &pfcpType.NetworkInstance{NetworkInstance: smContext.Dnn},
				}

				DLPDR.State = smf_context.RULE_UPDATE
				DLPDR.FAR.State = smf_context.RULE_UPDATE

				pdrList = append(pdrList, DLPDR)
				farList = append(farList, DLPDR.FAR)

				if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
					smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				}
			}
		}

		if err := smf_context.
			HandlePDUSessionResourceSetupResponseTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.Log.Errorf("Handle PDUSessionResourceSetupResponseTransfer failed: %+v", err)
		}
		sendPFCPModification = true
		smContext.SetState(smf_context.PFCPModification)
	case models.N2SmInfoType_PDU_RES_SETUP_FAIL:
		if err := smf_context.
			HandlePDUSessionResourceSetupUnsuccessfulTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.Log.Errorf("Handle PDUSessionResourceSetupResponseTransfer failed: %+v", err)
		}
	case models.N2SmInfoType_PDU_RES_REL_RSP:
		// remove an tunnel info
		smContext.Tunnel.ANInformation = struct {
			IPAddress net.IP
			TEID      uint32
		}{nil, 0}

		smContext.Log.Infoln("Handle N2 PDU Resource Release Response")
		if smContext.PDUSessionRelease_DUE_TO_DUP_PDU_ID {
			smContext.CheckState(smf_context.InActivePending)
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture
			smContext.Log.Infoln("Release_DUE_TO_DUP_PDU_ID: Send Update SmContext Response")
			response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED
			// If NAS layer is inActive, the context should be remove
			if smContext.CheckState(smf_context.InActive) {
				// Use go routine to send Notification to prevent blocking the handling process
				go sendSMContextStatusNotification(smContext)
				smf_context.RemoveSMContext(smContextRef)
			}
		} else { // normal case
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture

			if smContext.CheckState(smf_context.InActive) {
				// If N1 PDU Session Release Complete is received, smContext state is InActive.
				// Remove SMContext when receiving N2 PDU Resource Release Response.
				// Use go routine to send Notification to prevent blocking the handling process
				go sendSMContextStatusNotification(smContext)
				smf_context.RemoveSMContext(smContext.Ref)
			}
		}
	case models.N2SmInfoType_PATH_SWITCH_REQ:
		smContext.Log.Traceln("Handle Path Switch Request")
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.SetState(smf_context.ModificationPending)

		if err := smf_context.HandlePathSwitchRequestTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.Log.Errorf("Handle PathSwitchRequestTransfer: %+v", err)
		}

		if n2Buf, err := smf_context.BuildPathSwitchRequestAcknowledgeTransfer(smContext); err != nil {
			smContext.Log.Errorf("Build Path Switch Transfer Error(%+v)", err)
		} else {
			response.BinaryDataN2SmInformation = n2Buf
		}

		response.JsonData.N2SmInfoType = models.N2SmInfoType_PATH_SWITCH_REQ_ACK
		response.JsonData.N2SmInfo = &models.RefToBinaryData{
			ContentId: "PATH_SWITCH_REQ_ACK",
		}

		smContext.PendingUPF = make(smf_context.PendingUPF)
		for _, dataPath := range tunnel.DataPathPool {
			if dataPath.Activated {
				ANUPF := dataPath.FirstDPNode
				DLPDR := ANUPF.DownLinkTunnel.PDR

				pdrList = append(pdrList, DLPDR)
				farList = append(farList, DLPDR.FAR)

				if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
					smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				}
			}
		}

		sendPFCPModification = true
		smContext.SetState(smf_context.PFCPModification)
	case models.N2SmInfoType_PATH_SWITCH_SETUP_FAIL:
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.SetState(smf_context.ModificationPending)
		if err :=
			smf_context.HandlePathSwitchRequestSetupFailedTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.Log.Errorf("HandlePathSwitchRequestSetupFailedTransfer failed: %v", err)
		}
	case models.N2SmInfoType_HANDOVER_REQUIRED:
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture
		smContext.SetState(smf_context.ModificationPending)
		response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "Handover"}
	}

	switch smContextUpdateData.HoState {
	case models.HoState_PREPARING:
		smContext.Log.Traceln("In HoState_PREPARING")
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.SetState(smf_context.ModificationPending)
		smContext.HoState = models.HoState_PREPARING
		if err := smf_context.HandleHandoverRequiredTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.Log.Errorf("Handle HandoverRequiredTransfer failed: %+v", err)
		}
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ

		if n2Buf, err := smf_context.BuildPDUSessionResourceSetupRequestTransfer(smContext); err != nil {
			smContext.Log.Errorf("Build PDUSession Resource Setup Request Transfer Error(%s)", err.Error())
		} else {
			response.BinaryDataN2SmInformation = n2Buf
		}
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ
		response.JsonData.N2SmInfo = &models.RefToBinaryData{
			ContentId: "PDU_RES_SETUP_REQ",
		}
		response.JsonData.HoState = models.HoState_PREPARING
	case models.HoState_PREPARED:
		smContext.Log.Traceln("In HoState_PREPARED")
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.SetState(smf_context.ModificationPending)
		smContext.HoState = models.HoState_PREPARED
		response.JsonData.HoState = models.HoState_PREPARED
		if err :=
			smf_context.HandleHandoverRequestAcknowledgeTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.Log.Errorf("Handle HandoverRequestAcknowledgeTransfer failed: %+v", err)
		}

		// request UPF establish indirect forwarding path for DL
		if smContext.DLForwardingType == smf_context.IndirectForwarding {
			smContext.PendingUPF = make(smf_context.PendingUPF)
			ANUPF := smContext.IndirectForwardingTunnel.FirstDPNode
			IndirectForwardingPDR := smContext.IndirectForwardingTunnel.FirstDPNode.UpLinkTunnel.PDR

			pdrList = append(pdrList, IndirectForwardingPDR)
			farList = append(farList, IndirectForwardingPDR.FAR)

			if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
				smContext.PendingUPF[ANUPF.GetNodeIP()] = true
			}

			// release indirect forwading path
			if err := ANUPF.UPF.RemovePDR(IndirectForwardingPDR); err != nil {
				logger.PduSessLog.Errorln("release indirect path: ", err)
			}

			sendPFCPModification = true
			smContext.SetState(smf_context.PFCPModification)
		}

		if n2Buf, err := smf_context.BuildHandoverCommandTransfer(smContext); err != nil {
			smContext.Log.Errorf("Build HandoverCommandTransfer failed: %v", err)
		} else {
			response.BinaryDataN2SmInformation = n2Buf
		}

		response.JsonData.N2SmInfoType = models.N2SmInfoType_HANDOVER_CMD
		response.JsonData.N2SmInfo = &models.RefToBinaryData{
			ContentId: "HANDOVER_CMD",
		}
		response.JsonData.HoState = models.HoState_PREPARING
	case models.HoState_COMPLETED:
		smContext.Log.Traceln("In HoState_COMPLETED")
		smContext.CheckState(smf_context.Active)
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.PendingUPF = make(smf_context.PendingUPF)
		for _, dataPath := range tunnel.DataPathPool {
			if dataPath.Activated {
				ANUPF := dataPath.FirstDPNode
				DLPDR := ANUPF.DownLinkTunnel.PDR

				pdrList = append(pdrList, DLPDR)
				farList = append(farList, DLPDR.FAR)

				if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
					smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				}
			}
		}

		// remove indirect forwarding path
		if smContext.DLForwardingType == smf_context.IndirectForwarding {
			indirectForwardingPDR := smContext.IndirectForwardingTunnel.FirstDPNode.GetUpLinkPDR()
			indirectForwardingPDR.State = smf_context.RULE_REMOVE
			indirectForwardingPDR.FAR.State = smf_context.RULE_REMOVE
			pdrList = append(pdrList, indirectForwardingPDR)
			farList = append(farList, indirectForwardingPDR.FAR)
		}

		sendPFCPModification = true
		smContext.SetState(smf_context.PFCPModification)
		smContext.HoState = models.HoState_COMPLETED
		response.JsonData.HoState = models.HoState_COMPLETED
	}

	switch smContextUpdateData.Cause {
	case models.Cause_REL_DUE_TO_DUPLICATE_SESSION_ID:
		//* release PDU Session Here
		// Wait till the state becomes Active again
		// TODO: implement sleep wait in concurrent architecture

		smContext.Log.Infoln("[SMF] Cause_REL_DUE_TO_DUPLICATE_SESSION_ID")

		smContext.PDUSessionRelease_DUE_TO_DUP_PDU_ID = true

		switch smContext.State() {
		case smf_context.ActivePending, smf_context.ModificationPending, smf_context.Active:
			response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "PDUResourceReleaseCommand"}
			response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_REL_CMD

			if buf, err := smf_context.BuildPDUSessionResourceReleaseCommandTransfer(smContext); err != nil {
				smContext.Log.Errorf("Build PDUSessionResourceReleaseCommandTransfer failed: %v", err)
			} else {
				response.BinaryDataN2SmInformation = buf
			}

			smContext.SetState(smf_context.PFCPModification)
			releaseTunnel(smContext)
			sendPFCPDelete = true
		default:
			smContext.Log.Infof("Not needs to send pfcp release")
		}
	}

	// Check FSM and take corresponding action
	switch smContext.State() {
	case smf_context.PFCPModification:
		smContext.Log.Traceln("In case PFCPModification")

		if sendPFCPModification {
			defaultPath := smContext.Tunnel.DataPathPool.GetDefaultPath()
			ANUPF := defaultPath.FirstDPNode
			pfcp_message.SendPfcpSessionModificationRequest(
				ANUPF.UPF.NodeID, ANUPF.UPF.Addr, smContext, pdrList, farList, barList, qerList)
		}

		if sendPFCPDelete {
			smContext.Log.Infoln("Send PFCP Deletion from HandlePDUSessionSMContextUpdate")
		}

		switch smContext.WaitPFCPCommunicationStatus() {
		case smf_context.SessionUpdateSuccess:
			smContext.Log.Traceln("In case SessionUpdateSuccess")
			smContext.SetState(smf_context.Active)
			httpResponse = &httpwrapper.Response{
				Status: http.StatusOK,
				Body:   response,
			}
		case smf_context.SessionUpdateFailed:
			smContext.Log.Traceln("In case SessionUpdateFailed")
			smContext.SetState(smf_context.Active)
			// It is just a template
			httpResponse = &httpwrapper.Response{
				Status: http.StatusForbidden,
				Body: models.UpdateSmContextErrorResponse{
					JsonData: &models.SmContextUpdateError{
						Error: &Nsmf_PDUSession.N1SmError,
					},
				}, // Depends on the reason why N4 fail
			}

		case smf_context.SessionReleaseSuccess:
			smContext.Log.Traceln("In case SessionReleaseSuccess")
			smContext.SetState(smf_context.InActivePending)
			httpResponse = &httpwrapper.Response{
				Status: http.StatusOK,
				Body:   response,
			}

		case smf_context.SessionReleaseFailed:
			// Update SmContext Request(N1 PDU Session Release Request)
			// Send PDU Session Release Reject
			smContext.Log.Traceln("In case SessionReleaseFailed")
			problemDetail := models.ProblemDetails{
				Status: http.StatusInternalServerError,
				Cause:  "SYSTEM_FAILULE",
			}
			httpResponse = &httpwrapper.Response{
				Status: int(problemDetail.Status),
			}
			smContext.SetState(smf_context.Active)
			errResponse := models.UpdateSmContextErrorResponse{
				JsonData: &models.SmContextUpdateError{
					Error: &problemDetail,
				},
			}
			if buf, err := smf_context.BuildGSMPDUSessionReleaseReject(smContext); err != nil {
				smContext.Log.Errorf("build GSM PDUSessionReleaseReject failed: %+v", err)
			} else {
				errResponse.BinaryDataN1SmMessage = buf
			}

			errResponse.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionReleaseReject"}
			httpResponse.Body = errResponse
		}
	case smf_context.ModificationPending:
		smContext.Log.Traceln("In case ModificationPending")
		smContext.SetState(smf_context.Active)
		httpResponse = &httpwrapper.Response{
			Status: http.StatusOK,
			Body:   response,
		}
	case smf_context.InActive, smf_context.InActivePending:
		smContext.Log.Traceln("In case InActive, InActivePending")
		httpResponse = &httpwrapper.Response{
			Status: http.StatusOK,
			Body:   response,
		}

		if smContext.PDUSessionRelease_DUE_TO_DUP_PDU_ID {
			go sendSMContextStatusNotification(smContext)
			smf_context.RemoveSMContext(smContext.Ref)
		}

	default:
		httpResponse = &httpwrapper.Response{
			Status: http.StatusOK,
			Body:   response,
		}
	}

	return httpResponse
}

func sendSMContextStatusNotification(smContext *smf_context.SMContext) {
	// Use go routine to send Notification to prevent blocking the handling process
	problemDetails, err := consumer.SendSMContextStatusNotification(smContext.SmStatusNotifyUri)
	if problemDetails != nil || err != nil {
		if problemDetails != nil {
			smContext.Log.Warnf("Send SMContext Status Notification Problem[%+v]", problemDetails)
		}

		if err != nil {
			smContext.Log.Warnf("Send SMContext Status Notification Error[%v]", err)
		}
	} else {
		smContext.Log.Traceln("Send SMContext Status Notification successfully")
	}
}

func HandlePDUSessionSMContextRelease(smContextRef string, body models.ReleaseSmContextRequest) *httpwrapper.Response {
	logger.PduSessLog.Infoln("In HandlePDUSessionSMContextRelease")
	smContext := smf_context.GetSMContextByRef(smContextRef)

	if smContext == nil {
		logger.PduSessLog.Warnf("SMContext[%s] is not found", smContextRef)

		httpResponse := &httpwrapper.Response{
			Header: nil,
			Status: http.StatusNotFound,
			Body: models.UpdateSmContextErrorResponse{
				JsonData: &models.SmContextUpdateError{
					UpCnxState: models.UpCnxState_DEACTIVATED,
					Error: &models.ProblemDetails{
						Type:   "Resource Not Found",
						Title:  "SMContext Ref is not found",
						Status: http.StatusNotFound,
					},
				},
			},
		}
		return httpResponse
	}

	smContext.SMLock.Lock()
	defer smContext.SMLock.Unlock()

	smContext.StopT3591()
	smContext.StopT3592()

	// remove SM Policy Association
	if smContext.SMPolicyID != "" {
		if err := consumer.SendSMPolicyAssociationTermination(smContext); err != nil {
			smContext.Log.Errorf("SM Policy Termination failed: %s", err)
		} else {
			smContext.SMPolicyID = ""
		}
	}

	if !smContext.CheckState(smf_context.InActive) {
		smContext.SetState(smf_context.PFCPModification)
		releaseTunnel(smContext)
	} else {
		smContext.SendPFCPCommunicationStatus(smf_context.SessionReleaseSuccess)
	}

	var httpResponse *httpwrapper.Response

	switch PFCPResponseStatus := smContext.WaitPFCPCommunicationStatus(); PFCPResponseStatus {
	case smf_context.SessionReleaseSuccess:
		smContext.Log.Traceln("In case SessionReleaseSuccess")
		smContext.SetState(smf_context.InActive)
		httpResponse = &httpwrapper.Response{
			Status: http.StatusNoContent,
			Body:   nil,
		}

	case smf_context.SessionReleaseFailed:
		// Update SmContext Request(N1 PDU Session Release Request)
		// Send PDU Session Release Reject
		smContext.Log.Traceln("In case SessionReleaseFailed")
		problemDetail := models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "SYSTEM_FAILULE",
		}
		httpResponse = &httpwrapper.Response{
			Status: int(problemDetail.Status),
		}
		smContext.SetState(smf_context.Active)
		errResponse := models.UpdateSmContextErrorResponse{
			JsonData: &models.SmContextUpdateError{
				Error: &problemDetail,
			},
		}
		if buf, err := smf_context.BuildGSMPDUSessionReleaseReject(smContext); err != nil {
			smContext.Log.Errorf("Build GSM PDUSessionReleaseReject failed: %+v", err)
		} else {
			errResponse.BinaryDataN1SmMessage = buf
		}

		errResponse.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionReleaseReject"}
		httpResponse.Body = errResponse
	default:
		smContext.Log.Warnf("The state shouldn't be [%s]\n", PFCPResponseStatus)

		smContext.Log.Traceln("In case Unknown")
		problemDetail := models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "SYSTEM_FAILULE",
		}
		httpResponse = &httpwrapper.Response{
			Status: int(problemDetail.Status),
		}
		smContext.SetState(smf_context.Active)
		errResponse := models.UpdateSmContextErrorResponse{
			JsonData: &models.SmContextUpdateError{
				Error: &problemDetail,
			},
		}
		if buf, err := smf_context.BuildGSMPDUSessionReleaseReject(smContext); err != nil {
			smContext.Log.Errorf("Build GSM PDUSessionReleaseReject failed: %+v", err)
		} else {
			errResponse.BinaryDataN1SmMessage = buf
		}

		errResponse.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionReleaseReject"}
		httpResponse.Body = errResponse
	}

	smf_context.RemoveSMContext(smContext.Ref)

	return httpResponse
}

func HandlePDUSessionSMContextLocalRelease(smContext *smf_context.SMContext, createData *models.SmContextCreateData) {
	smContext.SMLock.Lock()
	defer smContext.SMLock.Unlock()

	// remove SM Policy Association
	if smContext.SMPolicyID != "" {
		if err := consumer.SendSMPolicyAssociationTermination(smContext); err != nil {
			logger.PduSessLog.Errorf("SM Policy Termination failed: %s", err)
		} else {
			smContext.SMPolicyID = ""
		}
	}

	smContext.SetState(smf_context.PFCPModification)

	releaseTunnel(smContext)

	switch PFCPResponseStatus := smContext.WaitPFCPCommunicationStatus(); PFCPResponseStatus {
	case smf_context.SessionReleaseSuccess:
		logger.CtxLog.Traceln("In case SessionReleaseSuccess")
		smContext.SetState(smf_context.InActivePending)
		if createData.SmContextStatusUri != smContext.SmStatusNotifyUri {
			problemDetails, err := consumer.SendSMContextStatusNotification(smContext.SmStatusNotifyUri)
			if problemDetails != nil || err != nil {
				if problemDetails != nil {
					logger.PduSessLog.Warnf("Send SMContext Status Notification Problem[%+v]", problemDetails)
				}

				if err != nil {
					logger.PduSessLog.Warnf("Send SMContext Status Notification Error[%v]", err)
				}
			} else {
				logger.PduSessLog.Traceln("Send SMContext Status Notification successfully")
			}
		}
		smf_context.RemoveSMContext(smContext.Ref)

	case smf_context.SessionReleaseFailed:
		logger.CtxLog.Traceln("In case SessionReleaseFailed")
		smContext.SetState(smf_context.Active)

	default:
		logger.CtxLog.Warnf("The state shouldn't be [%s]\n", PFCPResponseStatus)
		logger.CtxLog.Traceln("In case Unknown")
		smContext.SetState(smf_context.Active)
	}
}

func releaseTunnel(smContext *smf_context.SMContext) {
	deletedPFCPNode := make(map[string]bool)
	smContext.PendingUPF = make(smf_context.PendingUPF)
	for _, dataPath := range smContext.Tunnel.DataPathPool {
		if !dataPath.Activated {
			continue
		}
		dataPath.DeactivateTunnelAndPDR(smContext)
		for curDataPathNode := dataPath.FirstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {
			curUPFID, err := curDataPathNode.GetUPFID()
			if err != nil {
				logger.PduSessLog.Error(err)
				continue
			}
			if _, exist := deletedPFCPNode[curUPFID]; !exist {
				pfcp_message.SendPfcpSessionDeletionRequest(curDataPathNode.UPF.NodeID, curDataPathNode.UPF.Addr, smContext)
				deletedPFCPNode[curUPFID] = true
				smContext.PendingUPF[curDataPathNode.GetNodeIP()] = true
			}
		}
	}
}

func makeErrorResponse(smContext *smf_context.SMContext, nasErrorCause uint8,
	sbiError *models.ProblemDetails) *httpwrapper.Response {
	var httpResponse *httpwrapper.Response

	if buf, err := smf_context.
		BuildGSMPDUSessionEstablishmentReject(
			smContext,
			nasErrorCause); err != nil {
		httpResponse = &httpwrapper.Response{
			Header: nil,
			Status: int(sbiError.Status),
			Body: models.PostSmContextsErrorResponse{
				JsonData: &models.SmContextCreateError{
					Error:   sbiError,
					N1SmMsg: &models.RefToBinaryData{ContentId: "n1SmMsg"},
				},
			},
		}
	} else {
		httpResponse = &httpwrapper.Response{
			Header: nil,
			Status: int(sbiError.Status),
			Body: models.PostSmContextsErrorResponse{
				JsonData: &models.SmContextCreateError{
					Error:   sbiError,
					N1SmMsg: &models.RefToBinaryData{ContentId: "n1SmMsg"},
				},
				BinaryDataN1SmMessage: buf,
			},
		}
	}
	return httpResponse
}

func sendGSMPDUSessionReleaseCommand(smContext *smf_context.SMContext, nasPdu []byte) {
	n1n2Request := models.N1N2MessageTransferRequest{}
	n1n2Request.JsonData = &models.N1N2MessageTransferReqData{
		PduSessionId: smContext.PDUSessionID,
		N1MessageContainer: &models.N1MessageContainer{
			N1MessageClass:   "SM",
			N1MessageContent: &models.RefToBinaryData{ContentId: "GSM_NAS"},
		},
	}
	n1n2Request.BinaryDataN1Message = nasPdu
	if smContext.T3592 != nil {
		smContext.T3592.Stop()
		smContext.T3592 = nil
	}

	// Start T3592
	t3592 := factory.SmfConfig.Configuration.T3592
	if t3592.Enable {
		smContext.T3592 = smf_context.NewTimer(t3592.ExpireTime, t3592.MaxRetryTimes, func(expireTimes int32) {
			smContext.SMLock.Lock()
			rspData, rsp, err := smContext.
				CommunicationClient.
				N1N2MessageCollectionDocumentApi.
				N1N2MessageTransfer(context.Background(), smContext.Supi, n1n2Request)
			if err != nil {
				smContext.Log.Warnf("Send N1N2Transfer for GSMPDUSessionReleaseCommand failed: %s", err)
			}
			if rspData.Cause == models.N1N2MessageTransferCause_N1_MSG_NOT_TRANSFERRED {
				smContext.Log.Warnf("%v", rspData.Cause)
			}
			if err := rsp.Body.Close(); err != nil {
				smContext.Log.Warn("Close body failed", err)
			}
			smContext.SMLock.Unlock()
		}, func() {
			smContext.Log.Warn("T3592 Expires 3 times, abort notification procedure")
			smContext.SMLock.Lock()
			smContext.T3592 = nil
			sendSMContextStatusNotification(smContext)
			smContext.SMLock.Unlock()
		})
	}
}

func sendGSMPDUSessionModificationCommand(smContext *smf_context.SMContext, nasPdu []byte) {
	n1n2Request := models.N1N2MessageTransferRequest{}
	n1n2Request.JsonData = &models.N1N2MessageTransferReqData{
		PduSessionId: smContext.PDUSessionID,
		N1MessageContainer: &models.N1MessageContainer{
			N1MessageClass:   "SM",
			N1MessageContent: &models.RefToBinaryData{ContentId: "GSM_NAS"},
		},
	}
	n1n2Request.BinaryDataN1Message = nasPdu

	if smContext.T3591 != nil {
		smContext.T3591.Stop()
		smContext.T3591 = nil
	}

	// Start T3591
	t3591 := factory.SmfConfig.Configuration.T3591
	if t3591.Enable {
		smContext.T3591 = smf_context.NewTimer(t3591.ExpireTime, t3591.MaxRetryTimes, func(expireTimes int32) {
			smContext.SMLock.Lock()
			defer smContext.SMLock.Unlock()
			rspData, rsp, err := smContext.
				CommunicationClient.
				N1N2MessageCollectionDocumentApi.
				N1N2MessageTransfer(context.Background(), smContext.Supi, n1n2Request)
			if err != nil {
				smContext.Log.Warnf("Send N1N2Transfer for GSMPDUSessionModificationCommand failed: %s", err)
			}
			if rspData.Cause == models.N1N2MessageTransferCause_N1_MSG_NOT_TRANSFERRED {
				smContext.Log.Warnf("%v", rspData.Cause)
			}
			if err := rsp.Body.Close(); err != nil {
				smContext.Log.Warn("Close body failed", err)
			}
		}, func() {
			smContext.Log.Warn("T3591 Expires3 times, abort notification procedure")
			smContext.SMLock.Lock()
			defer smContext.SMLock.Unlock()
			smContext.T3591 = nil
		})
	}
}