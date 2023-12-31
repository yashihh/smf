package oam

import (
	"github.com/gin-gonic/gin"

	"bitbucket.org/free5gc-team/smf/internal/sbi/producer"
	"bitbucket.org/free5gc-team/util/httpwrapper"
)

func HTTPGetUEPDUSessionInfo(c *gin.Context) {
	req := httpwrapper.NewRequest(c.Request, nil)
	req.Params["smContextRef"] = c.Params.ByName("smContextRef")

	smContextRef := req.Params["smContextRef"]
	HTTPResponse := producer.HandleOAMGetUEPDUSessionInfo(smContextRef)

	c.JSON(HTTPResponse.Status, HTTPResponse.Body)
}
