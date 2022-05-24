package mdaf

import (
	"net/http"

	"loadbalance/logger"

	"github.com/free5gc/openapi/models"
)

func MDAFProcedure(request models.Amf3GppAccessRegistration) (header http.Header, response *models.Amf3GppAccessRegistration, problemDetails *models.ProblemDetails) {
	logger.HttpLog.Infoln("========MDAFProcedure========")

	amfNum := request.AmfInstanceId
	ueNum := request.SupportedFeatures
	cpuRate := request.Pei
	logger.HttpLog.Infof("amfNum: %v, ueNum: %v, cpuRate: %v", amfNum, ueNum, cpuRate)
	header = make(http.Header)
	header.Set("Location", "http://127.0.0.21:8000/nmdaf-msg/v1")
	return header, &request, nil
}
