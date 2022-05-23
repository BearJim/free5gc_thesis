package mdaf

import (
	"net/http"

	"loadbalance/logger"

	"github.com/free5gc/openapi/models"
)

func MDAFProcedure(request amfData) (header http.Header, response *amfData, problemDetails *models.ProblemDetails) {

	logger.HttpLog.Infoln("========MDAFProcedure========")
	header = make(http.Header)
	header.Set("Location", "http://127.0.0.21:8000/nmdaf-msg/v1")
	return header, &request, nil
}
