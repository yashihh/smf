package consumer

import (
	"context"
	"fmt"
	"free5gc/lib/openapi"
	"free5gc/lib/openapi/Nnrf_NFDiscovery"
	"free5gc/lib/openapi/Nudm_SubscriberDataManagement"
	"free5gc/lib/openapi/models"
	smf_context "free5gc/src/smf/context"
	"free5gc/src/smf/logger"
	"net/http"

	"strings"
	"time"

	"github.com/antihax/optional"
	"github.com/mohae/deepcopy"
)

func SendNFRegistration() error {

	//set nfProfile
	profile := models.NfProfile{
		NfInstanceId:  smf_context.SMF_Self().NfInstanceID,
		NfType:        models.NfType_SMF,
		NfStatus:      models.NfStatus_REGISTERED,
		Ipv4Addresses: []string{smf_context.SMF_Self().HTTPAddress},
		NfServices:    smf_context.NFServices,
		SmfInfo:       smf_context.SmfInfo,
	}
	var rep models.NfProfile
	var res *http.Response
	var err error

	// Check data (Use RESTful PUT)
	for {
		rep, res, err = smf_context.SMF_Self().NFManagementClient.NFInstanceIDDocumentApi.RegisterNFInstance(context.TODO(), smf_context.SMF_Self().NfInstanceID, profile)
		if err != nil || res == nil {
			logger.AppLog.Infof("SMF register to NRF Error[%s]", err.Error())
			time.Sleep(2 * time.Second)
			continue
		}

		status := res.StatusCode
		if status == http.StatusOK {
			// NFUpdate
			break
		} else if status == http.StatusCreated {
			// NFRegister
			resourceUri := res.Header.Get("Location")
			// resouceNrfUri := resourceUri[strings.LastIndex(resourceUri, "/"):]
			smf_context.SMF_Self().NfInstanceID = resourceUri[strings.LastIndex(resourceUri, "/")+1:]
			break
		} else {
			logger.AppLog.Infof("handler returned wrong status code %d", status)
			// fmt.Errorf("NRF return wrong status code %d", status)
		}
	}

	logger.InitLog.Infof("SMF Registration to NRF %v", rep)
	return nil
}

func RetrySendNFRegistration(MaxRetry int) error {

	retryCount := 0
	for retryCount < MaxRetry {
		err := SendNFRegistration()
		if err == nil {
			return nil
		}
		logger.AppLog.Warnf("Send NFRegistration Failed by %v", err)
		retryCount++
	}

	return fmt.Errorf("[SMF] Retry NF Registration has meet maximum")
}

func SendNFDeregistration() (err error) {

	// Check data (Use RESTful DELETE)
	res, localErr := smf_context.SMF_Self().NFManagementClient.NFInstanceIDDocumentApi.DeregisterNFInstance(context.TODO(), smf_context.SMF_Self().NfInstanceID)
	if localErr != nil {
		logger.AppLog.Warnln(localErr)
		err = localErr
		return
	}
	if res != nil {
		if status := res.StatusCode; status != http.StatusNoContent {
			logger.AppLog.Warnln("handler returned wrong status code ", status)
			err = openapi.ReportError("handler returned wrong status code ", status)
		}
	}

	return

}

func SendNFDiscoveryUDM() (problemDetails *models.ProblemDetails, err error) {
	if smf_context.SMF_Self().SubscriberDataManagementClient != nil {
		return
	}

	localVarOptionals := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{}

	// Check data
	result, httpResp, localErr := smf_context.SMF_Self().NFDiscoveryClient.NFInstancesStoreApi.SearchNFInstances(context.TODO(), models.NfType_UDM, models.NfType_SMF, &localVarOptionals)

	if localErr == nil {
		smf_context.SMF_Self().UDMProfile = result.NfInstances[0]

		for _, service := range *smf_context.SMF_Self().UDMProfile.NfServices {
			if service.ServiceName == models.ServiceName_NUDM_SDM {
				SDMConf := Nudm_SubscriberDataManagement.NewConfiguration()
				SDMConf.SetBasePath(service.ApiPrefix)
				smf_context.SMF_Self().SubscriberDataManagementClient = Nudm_SubscriberDataManagement.NewAPIClient(SDMConf)
			}
		}

		if smf_context.SMF_Self().SubscriberDataManagementClient == nil {
			logger.AppLog.Warnln("sdm client failed")
		}
	} else if httpResp != nil {
		logger.AppLog.Warnln("handler returned wrong status code ", httpResp.Status)
		if httpResp.Status != localErr.Error() {
			err = localErr
			return
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return
}

func SendNFDiscoveryPCF() (problemDetails *models.ProblemDetails, err error) {

	// Set targetNfType
	targetNfType := models.NfType_PCF
	// Set requestNfType
	requesterNfType := models.NfType_SMF
	localVarOptionals := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{}

	// Check data
	result, httpResp, localErr := smf_context.SMF_Self().NFDiscoveryClient.NFInstancesStoreApi.SearchNFInstances(context.TODO(), targetNfType, requesterNfType, &localVarOptionals)

	if localErr == nil {
		logger.AppLog.Traceln(result.NfInstances)
	} else if httpResp != nil {
		logger.AppLog.Warnln("handler returned wrong status code ", httpResp.Status)
		if httpResp.Status != localErr.Error() {
			err = localErr
			return
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}

	return
}

func SendNFDiscoveryServingAMF(smContext *smf_context.SMContext) (problemDetails *models.ProblemDetails, err error) {
	targetNfType := models.NfType_AMF
	requesterNfType := models.NfType_SMF

	localVarOptionals := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{}

	localVarOptionals.TargetNfInstanceId = optional.NewInterface(smContext.ServingNfId)

	// Check data
	result, httpResp, localErr := smf_context.SMF_Self().NFDiscoveryClient.NFInstancesStoreApi.SearchNFInstances(context.TODO(), targetNfType, requesterNfType, &localVarOptionals)

	if localErr == nil {
		if result.NfInstances == nil {
			if status := httpResp.StatusCode; status != http.StatusOK {
				logger.AppLog.Warnln("handler returned wrong status code", status)
			}
			logger.AppLog.Warnln("NfInstances is nil")
			err = openapi.ReportError("NfInstances is nil")
			return
		}
		logger.AppLog.Info("SendNFDiscoveryServingAMF ok")
		smContext.AMFProfile = deepcopy.Copy(result.NfInstances[0]).(models.NfProfile)
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}

	return

}

func SendDeregisterNFInstance() (problemDetails *models.ProblemDetails, err error) {
	logger.AppLog.Infof("Send Deregister NFInstance")

	smfSelf := smf_context.SMF_Self()
	// Set client and set url

	var res *http.Response

	res, err = smfSelf.NFManagementClient.NFInstanceIDDocumentApi.DeregisterNFInstance(context.Background(), smfSelf.NfInstanceID)
	if err == nil {
		return
	} else if res != nil {
		if res.Status != err.Error() {
			return
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return
}
