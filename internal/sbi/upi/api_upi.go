package upi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	smf_context "bitbucket.org/free5gc-team/smf/internal/context"
	"bitbucket.org/free5gc-team/smf/pkg/association"
	"bitbucket.org/free5gc-team/smf/pkg/factory"
	"bitbucket.org/free5gc-team/util/httpwrapper"
)

func GetUpNodesLinks(c *gin.Context) {
	upi := smf_context.SMF_Self().UserPlaneInformation
	nodes := upi.UpNodesToConfiguration()
	links := upi.LinksToConfiguration()

	json := &factory.UserPlaneInformation{
		UPNodes: nodes,
		Links:   links,
	}

	httpResponse := &httpwrapper.Response{
		Header: nil,
		Status: http.StatusOK,
		Body:   json,
	}
	c.JSON(httpResponse.Status, httpResponse.Body)
}

func PostUpNodesLinks(c *gin.Context) {
	upi := smf_context.SMF_Self().UserPlaneInformation
	var json factory.UserPlaneInformation
	if err := c.ShouldBindJSON(&json); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	upi.UpNodesFromConfiguration(&json)
	upi.LinksFromConfiguration(&json)

	for _, upf := range upi.UPFs {
		// only associate new ones
		if upf.UPF.UPFStatus == smf_context.NotAssociated {
			go association.ToBeAssociatedWithUPF(smf_context.SMF_Self().Ctx, upf.UPF)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "OK"})
}
