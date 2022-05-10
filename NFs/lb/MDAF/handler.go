package mdaf

import (
	"net/http"

	"github.com/free5gc/openapi/models"
)

func MDAFProcedure(request amfData) (header http.Header, response *models.Amf3GppAccessRegistration,
	problemDetails *models.ProblemDetails) {
	// caculate goAmf
	return
}
