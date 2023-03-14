package producer_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"

	"bitbucket.org/free5gc-team/nas"
	"bitbucket.org/free5gc-team/nas/nasMessage"
	"bitbucket.org/free5gc-team/nas/nasType"
	"bitbucket.org/free5gc-team/openapi"
	"bitbucket.org/free5gc-team/openapi/Nsmf_PDUSession"
	"bitbucket.org/free5gc-team/openapi/models"
	"bitbucket.org/free5gc-team/smf/internal/context"
	"bitbucket.org/free5gc-team/smf/internal/pfcp"
	"bitbucket.org/free5gc-team/smf/internal/pfcp/udp"
	"bitbucket.org/free5gc-team/smf/internal/sbi/producer"
	"bitbucket.org/free5gc-team/smf/pkg/factory"
	"bitbucket.org/free5gc-team/util/httpwrapper"
)

var userPlaneConfig = factory.UserPlaneInformation{
	UPNodes: map[string]*factory.UPNode{
		"GNodeB": {
			Type: "AN",
		},
		"UPF1": {
			Type:   "UPF",
			NodeID: "192.168.179.1",
			SNssaiInfos: []*factory.SnssaiUpfInfoItem{
				{
					SNssai: &models.Snssai{
						Sst: 1,
						Sd:  "112232",
					},
					DnnUpfInfoList: []*factory.DnnUpfInfoItem{
						{
							Dnn: "internet",
							Pools: []*factory.UEIPPool{
								{Cidr: "10.60.0.0/16"},
							},
						},
					},
				},
			},
			InterfaceUpfInfoList: []*factory.InterfaceUpfInfoItem{
				{
					InterfaceType: "N3",
					Endpoints: []string{
						"127.0.0.8",
					},
					NetworkInstances: []string{"internet"},
				},
			},
		},
	},
	Links: []*factory.UPLink{
		{
			A: "GNodeB",
			B: "UPF1",
		},
	},
}

var testConfig = factory.Config{
	Info: &factory.Info{
		Version:     "1.0.0",
		Description: "SMF procdeure test configuration",
	},
	Configuration: &factory.Configuration{
		SmfName: "SMF Procedure Test",
		Sbi: &factory.Sbi{
			Scheme:       "http",
			RegisterIPv4: "127.0.0.1",
			BindingIPv4:  "127.0.0.1",
			Port:         8000,
		},
		PFCP: &factory.PFCP{
			ListenAddr:   "127.0.0.1",
			ExternalAddr: "127.0.0.1",
			NodeID:       "127.0.0.1",
		},
		NrfUri:               "http://127.0.0.10:8000",
		UserPlaneInformation: userPlaneConfig,
		ServiceNameList: []string{
			"nsmf-pdusession",
			"nsmf-event-exposure",
			"nsmf-oam",
		},
		SNssaiInfo: []*factory.SnssaiInfoItem{
			{
				SNssai: &models.Snssai{
					Sst: 1,
					Sd:  "112232",
				},
				DnnInfos: []*factory.SnssaiDnnInfoItem{
					{
						Dnn: "internet",
						DNS: &factory.DNS{
							IPv4Addr: "8.8.8.8",
							IPv6Addr: "2001:4860:4860::8888",
						},
					},
				},
			},
		},
	},
}

func initConfig() {
	context.InitSmfContext(&testConfig)
	factory.SmfConfig = testConfig
}

func initDiscUDMStubNRF() {
	searchResult := &models.SearchResult{
		ValidityPeriod: 100,
		NfInstances: []models.NfProfile{
			{
				NfInstanceId: "smf-unit-testing",
				NfType:       "UDM",
				NfStatus:     "REGISTERED",
				PlmnList: &[]models.PlmnId{
					{
						Mcc: "208",
						Mnc: "93",
					},
				},
				Ipv4Addresses: []string{
					"127.0.0.3",
				},
				NfServices: &[]models.NfService{
					{
						ServiceInstanceId: "0",
						ServiceName:       "nudm-sdm",
						Versions: &[]models.NfServiceVersion{
							{
								ApiVersionInUri: "v1",
								ApiFullVersion:  "1.0.0",
							},
						},
						Scheme:          "http",
						NfServiceStatus: "REGISTERED",
						IpEndPoints: &[]models.IpEndPoint{
							{
								Ipv4Address: "127.0.0.3",
								Transport:   "TCP",
								Port:        8000,
							},
						},
						ApiPrefix: "http://127.0.0.3:8000",
					},
				},
			},
		},
	}

	gock.New("http://127.0.0.10:8000/nnrf-disc/v1").
		Get("/nf-instances").
		MatchParam("target-nf-type", "UDM").
		MatchParam("requester-nf-type", "SMF").
		Reply(http.StatusOK).
		JSON(searchResult)
}

func initDiscPCFStubNRF() {
	searchResult := &models.SearchResult{
		ValidityPeriod: 100,
		NfInstances: []models.NfProfile{
			{
				NfInstanceId: "smf-unit-testing",
				NfType:       "PCF",
				NfStatus:     "REGISTERED",
				PlmnList: &[]models.PlmnId{
					{
						Mcc: "208",
						Mnc: "93",
					},
				},
				Ipv4Addresses: []string{
					"127.0.0.7",
				},
				PcfInfo: &models.PcfInfo{
					DnnList: []string{
						"free5gc",
						"internet",
					},
				},
				NfServices: &[]models.NfService{
					{
						ServiceInstanceId: "1",
						ServiceName:       "npcf-smpolicycontrol",
						Versions: &[]models.NfServiceVersion{
							{
								ApiVersionInUri: "v1",
								ApiFullVersion:  "1.0.0",
							},
						},
						Scheme:          "http",
						NfServiceStatus: "REGISTERED",
						IpEndPoints: &[]models.IpEndPoint{
							{
								Ipv4Address: "127.0.0.7",
								Transport:   "TCP",
								Port:        8000,
							},
						},
						ApiPrefix: "http://127.0.0.7:8000",
					},
				},
			},
		},
	}

	gock.New("http://127.0.0.10:8000/nnrf-disc/v1").
		Get("/nf-instances").
		MatchParam("target-nf-type", "PCF").
		MatchParam("requester-nf-type", "SMF").
		Reply(http.StatusOK).
		JSON(searchResult)
}

func initGetSMDataStubUDM() {
	SMSubscriptionData := []models.SessionManagementSubscriptionData{
		{
			SingleNssai: &models.Snssai{
				Sst: 1,
				Sd:  "112232",
			},
			DnnConfigurations: map[string]models.DnnConfiguration{
				"internet": {
					PduSessionTypes: &models.PduSessionTypes{
						DefaultSessionType: "IPV4",
						AllowedSessionTypes: []models.PduSessionType{
							"IPV4",
						},
					},
					SscModes: &models.SscModes{
						DefaultSscMode: "SSC_MODE_1",
						AllowedSscModes: []models.SscMode{
							"SSC_MODE_1",
							"SSC_MODE_2",
							"SSC_MODE_3",
						},
					},
					Var5gQosProfile: &models.SubscribedDefaultQos{
						Var5qi: 9,
						Arp: &models.Arp{
							PriorityLevel: 8,
						},
						PriorityLevel: 8,
					},
					SessionAmbr: &models.Ambr{
						Uplink:   "1000 Kbps",
						Downlink: "1000 Kbps",
					},
				},
			},
		},
	}

	gock.New("http://127.0.0.3:8000/nudm-sdm/v1/imsi-208930000007487").
		Get("/sm-data").
		MatchParam("dnn", "internet").
		Reply(http.StatusOK).
		JSON(SMSubscriptionData)
}

func initSMPoliciesPostStubPCF() {
	smPolicyDecision := models.SmPolicyDecision{
		SessRules: map[string]*models.SessionRule{
			"SessRuleId-10": {
				AuthSessAmbr: &models.Ambr{
					Uplink:   "1000 Kbps",
					Downlink: "1000 Kbps",
				},
				AuthDefQos: &models.AuthorizedDefaultQos{
					Var5qi: 9,
					Arp: &models.Arp{
						PriorityLevel: 8,
					},
					PriorityLevel: 8,
				},
				SessRuleId: "SessRuleId-10",
			},
		},
		PolicyCtrlReqTriggers: []models.PolicyControlRequestTrigger{
			"PLMN_CH", "RES_MO_RE", "AC_TY_CH", "UE_IP_CH", "PS_DA_OFF",
			"DEF_QOS_CH", "SE_AMBR_CH", "QOS_NOTIF", "RAT_TY_CH",
		},
		SuppFeat: "000f",
	}

	gock.New("http://127.0.0.7:8000/npcf-smpolicycontrol/v1").
		Post("/sm-policies").
		Reply(http.StatusCreated).
		AddHeader("Location", "http://127.0.0.7:8000/npcf-smpolicycontrol/v1/sm-policies/imsi-208930000007487-10").
		JSON(smPolicyDecision)
}

func initDiscAMFStubNRF() {
	searchResult := &models.SearchResult{
		ValidityPeriod: 100,
		NfInstances: []models.NfProfile{
			{
				NfInstanceId: "smf-unit-testing",
				NfType:       "AMF",
				NfStatus:     "REGISTERED",
				PlmnList: &[]models.PlmnId{
					{
						Mcc: "208",
						Mnc: "93",
					},
				},
				Ipv4Addresses: []string{
					"127.0.0.18",
				},
				AmfInfo: &models.AmfInfo{
					AmfSetId:    "3f8",
					AmfRegionId: "ca",
				},
				NfServices: &[]models.NfService{
					{
						ServiceInstanceId: "0",
						ServiceName:       "namf-comm",
						Versions: &[]models.NfServiceVersion{
							{
								ApiVersionInUri: "v1",
								ApiFullVersion:  "1.0.0",
							},
						},
						Scheme:          "http",
						NfServiceStatus: "REGISTERED",
						IpEndPoints: &[]models.IpEndPoint{
							{
								Ipv4Address: "127.0.0.18",
								Transport:   "TCP",
								Port:        8000,
							},
						},
						ApiPrefix: "http://127.0.0.18:8000",
					},
				},
			},
		},
	}

	gock.New("http://127.0.0.10:8000/nnrf-disc/v1").
		Get("/nf-instances").
		MatchParam("target-nf-type", "AMF").
		MatchParam("requester-nf-type", "SMF").
		Reply(http.StatusOK).
		JSON(searchResult)
}

func initStubPFCP() {
	udp.Run(pfcp.Dispatch)
}

func buildPDUSessionEstablishmentRequest(pduSessID uint8, PTI uint8, pduType uint8) []byte {
	msg := nas.NewMessage()
	msg.GsmMessage = nas.NewGsmMessage()
	msg.GsmMessage.PDUSessionEstablishmentRequest = nasMessage.NewPDUSessionEstablishmentRequest(0)
	msg.GsmHeader.SetMessageType(nas.MsgTypePDUSessionEstablishmentRequest)
	msg.GsmHeader.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSSessionManagementMessage)

	pduEstReq := msg.GsmMessage.PDUSessionEstablishmentRequest
	// Set GSM Message
	pduEstReq.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSSessionManagementMessage)
	pduEstReq.SetPDUSessionID(pduSessID)
	pduEstReq.SetPTI(PTI)
	pduEstReq.SetMessageType(nas.MsgTypePDUSessionEstablishmentRequest)
	pduEstReq.PDUSessionType = nasType.NewPDUSessionType(nasMessage.PDUSessionEstablishmentRequestPDUSessionTypeType)
	pduEstReq.PDUSessionType.SetPDUSessionTypeValue(pduType)

	if b, err := msg.PlainNasEncode(); err != nil {
		panic(err)
	} else {
		return b
	}
}

func buildPDUSessionModificationRequest(pduSessID uint8, PTI uint8) []byte {
	msg := nas.NewMessage()
	msg.GsmMessage = nas.NewGsmMessage()
	msg.GsmMessage.PDUSessionModificationRequest = nasMessage.NewPDUSessionModificationRequest(0)
	msg.GsmHeader.SetMessageType(nas.MsgTypePDUSessionModificationRequest)
	msg.GsmHeader.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSSessionManagementMessage)

	pduModReq := msg.GsmMessage.PDUSessionModificationRequest
	// Set GSM Message
	pduModReq.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSSessionManagementMessage)
	pduModReq.SetPDUSessionID(pduSessID)
	pduModReq.SetPTI(PTI)
	pduModReq.SetMessageType(nas.MsgTypePDUSessionModificationRequest)

	if b, err := msg.PlainNasEncode(); err != nil {
		panic(err)
	} else {
		return b
	}
}

func buildPDUSessionEstablishmentReject(pduSessID uint8, PTI uint8, cause uint8) []byte {
	msg := nas.NewMessage()
	msg.GsmMessage = nas.NewGsmMessage()
	msg.GsmMessage.PDUSessionEstablishmentReject = nasMessage.NewPDUSessionEstablishmentReject(0)
	msg.GsmHeader.SetMessageType(nas.MsgTypePDUSessionEstablishmentReject)
	msg.GsmHeader.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSSessionManagementMessage)

	pduEstRej := msg.GsmMessage.PDUSessionEstablishmentReject
	// Set GSM Message
	pduEstRej.SetExtendedProtocolDiscriminator(nasMessage.Epd5GSSessionManagementMessage)
	pduEstRej.SetPDUSessionID(pduSessID)
	pduEstRej.SetPTI(PTI)
	pduEstRej.SetMessageType(nas.MsgTypePDUSessionEstablishmentReject)
	pduEstRej.Cause5GSM.SetCauseValue(cause)

	if b, err := msg.PlainNasEncode(); err != nil {
		panic(err)
	} else {
		return b
	}
}

func TestHandlePDUSessionSMContextCreate(t *testing.T) {
	// Activate Gock
	openapi.InterceptH2CClient()
	defer openapi.RestoreH2CClient()
	initConfig()
	initStubPFCP()

	// modify associate setup status
	allUPFs := context.SMF_Self().UserPlaneInformation.UPFs
	for _, upfNode := range allUPFs {
		upfNode.UPF.UPFStatus = context.AssociatedSetUpSuccess
	}

	testCases := []struct {
		initFuncs       []func()
		request         models.PostSmContextsRequest
		paramStr        string
		resultStr       string
		expectedHTTPRsp *httpwrapper.Response
	}{
		{
			initFuncs: []func(){initDiscUDMStubNRF, initDiscPCFStubNRF, initSMPoliciesPostStubPCF, initDiscAMFStubNRF},
			request: models.PostSmContextsRequest{
				BinaryDataN1SmMessage: buildPDUSessionModificationRequest(10, 1),
			},
			paramStr:  "input wrong GSM Message type\n",
			resultStr: "PDUSessionSMContextCreate should fail due to wrong GSM type\n",
			expectedHTTPRsp: &httpwrapper.Response{
				Header: nil,
				Status: http.StatusForbidden,
				Body: models.PostSmContextsErrorResponse{
					JsonData: &models.SmContextCreateError{
						Error: &Nsmf_PDUSession.N1SmError,
					},
				},
			},
		},
		{
			initFuncs: []func(){
				initDiscUDMStubNRF, initDiscPCFStubNRF,
				initGetSMDataStubUDM, initSMPoliciesPostStubPCF, initDiscAMFStubNRF,
			},
			request: models.PostSmContextsRequest{
				JsonData: &models.SmContextCreateData{
					Supi:         "imsi-208930000007487",
					Pei:          "imeisv-1110000000000000",
					Gpsi:         "msisdn-0900000000",
					PduSessionId: 10,
					Dnn:          "internet",
					SNssai: &models.Snssai{
						Sst: 1,
						Sd:  "112232",
					},
					ServingNfId: "c8d0ee65-f466-48aa-a42f-235ec771cb52",
					Guami: &models.Guami{
						PlmnId: &models.PlmnId{
							Mcc: "208",
							Mnc: "93",
						},
						AmfId: "cafe00",
					},
					AnType: "3GPP_ACCESS",
					ServingNetwork: &models.PlmnId{
						Mcc: "208",
						Mnc: "93",
					},
				},
				BinaryDataN1SmMessage: buildPDUSessionEstablishmentRequest(10, 2, nasMessage.PDUSessionTypeIPv6),
			},
			paramStr:  "try request the IPv6 PDU session\n",
			resultStr: "Reject IPv6 PDU Session and respond error\n",
			expectedHTTPRsp: &httpwrapper.Response{
				Header: nil,
				Status: http.StatusForbidden,
				Body: models.PostSmContextsErrorResponse{
					JsonData: &models.SmContextCreateError{
						Error: &models.ProblemDetails{
							Title:  "Invalid N1 Message",
							Status: http.StatusForbidden,
							Detail: "N1 Message Error",
							Cause:  "N1_SM_ERROR",
						},
						N1SmMsg: &models.RefToBinaryData{ContentId: "n1SmMsg"},
					},
					BinaryDataN1SmMessage: buildPDUSessionEstablishmentReject(
						10, 2, nasMessage.Cause5GSMPDUSessionTypeIPv4OnlyAllowed),
				},
			},
		},
		{
			initFuncs: []func(){
				initDiscUDMStubNRF, initDiscPCFStubNRF,
				initGetSMDataStubUDM, initSMPoliciesPostStubPCF, initDiscAMFStubNRF,
			},
			request: models.PostSmContextsRequest{
				JsonData: &models.SmContextCreateData{
					Supi:         "imsi-208930000007487",
					Pei:          "imeisv-1110000000000000",
					Gpsi:         "msisdn-0900000000",
					PduSessionId: 10,
					Dnn:          "internet",
					SNssai: &models.Snssai{
						Sst: 1,
						Sd:  "112232",
					},
					ServingNfId: "c8d0ee65-f466-48aa-a42f-235ec771cb52",
					Guami: &models.Guami{
						PlmnId: &models.PlmnId{
							Mcc: "208",
							Mnc: "93",
						},
						AmfId: "cafe00",
					},
					AnType: "3GPP_ACCESS",
					ServingNetwork: &models.PlmnId{
						Mcc: "208",
						Mnc: "93",
					},
				},
				BinaryDataN1SmMessage: buildPDUSessionEstablishmentRequest(10, 3, nasMessage.PDUSessionTypeIPv4),
			},
			paramStr:  "input correct PostSmContexts Request\n",
			resultStr: "PDUSessionSMContextCreate should pass\n",
			expectedHTTPRsp: &httpwrapper.Response{
				Header: nil,
				Status: http.StatusCreated,
				Body: models.PostSmContextsResponse{
					JsonData: &models.SmContextCreatedData{
						SNssai: &models.Snssai{
							Sst: 1,
							Sd:  "112232",
						},
					},
				},
			},
		},
	}

	// init all stubs for all TCs first to prevent gock race condition
	for _, tc := range testCases {
		for _, initFunc := range tc.initFuncs {
			initFunc()
		}
	}

	for _, tc := range testCases {
		t.Run(tc.paramStr, func(t *testing.T) {
			httpResp := producer.HandlePDUSessionSMContextCreate(tc.request)

			require.Equal(t, tc.expectedHTTPRsp.Status, httpResp.Status)
			require.Equal(t, tc.expectedHTTPRsp.Body, httpResp.Body)

			createData := tc.request.JsonData
			if createData != nil {
				if ref, err := context.ResolveRef(createData.Supi,
					createData.PduSessionId); err == nil {
					context.RemoveSMContext(ref)
				}
			}
		})
	}

	err := udp.Server.Close()
	require.NoError(t, err)
}
