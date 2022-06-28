package mdaf

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"loadbalance/logger"

	"github.com/free5gc/http_wrapper"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
)

// type amfData struct {
// 	amfNum  string `json:"amfId" bson:"amfId"`
// 	ueNum   string `json:"ueNum" bson:"UeNum"`
// 	cpuRate string `json:"cpuRate" bson:"cpuRate"`
// }

func HTTPNotifyAmf(ctx *gin.Context) {
	logger.HttpLog.Debugln("===Start HTTPNotifyAmf===")
	// var amfData amfData
	var amf3GppAccessRegistration models.Amf3GppAccessRegistration

	// step 1: retrieve http request body
	requestBody, err := ctx.GetRawData()
	if err != nil {
		problemDetail := models.ProblemDetails{
			Title:  "System failure",
			Status: http.StatusInternalServerError,
			Detail: err.Error(),
			Cause:  "SYSTEM_FAILURE",
		}
		logger.HttpLog.Errorf("Get Request Body error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, problemDetail)
		return
	}

	// step 2: convert requestBody to openapi models
	// err = openapi.Deserialize(&amfData, requestBody, "application/json")
	err = openapi.Deserialize(&amf3GppAccessRegistration, requestBody, "application/json")
	if err != nil {
		problemDetail := "[Request Body] " + err.Error()
		rsp := models.ProblemDetails{
			Title:  "Malformed request syntax",
			Status: http.StatusBadRequest,
			Detail: problemDetail,
		}
		logger.HttpLog.Errorln(problemDetail)
		ctx.JSON(http.StatusBadRequest, rsp)
		return
	}
	// logger.AppLog.Infoln("~~amfData: ", amfData)
	logger.AppLog.Debugln("AMF data Info msg: ", amf3GppAccessRegistration)

	// req := http_wrapper.NewRequest(ctx.Request, amfData)
	req := http_wrapper.NewRequest(ctx.Request, amf3GppAccessRegistration)
	logger.AppLog.Debugln("===Start HandleHTTPNotifyAmf===")
	rsp := HandleHTTPNotifyAmf(req)

	// step 5: response
	for key, val := range rsp.Header { // header response is optional
		ctx.Header(key, val[0])
	}
	responseBody, err := openapi.Serialize(rsp.Body, "application/json")
	if err != nil {
		logger.HttpLog.Errorln(err)
		problemDetails := models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "SYSTEM_FAILURE",
			Detail: err.Error(),
		}
		ctx.JSON(http.StatusInternalServerError, problemDetails)
	} else {
		ctx.Data(rsp.Status, "application/json", responseBody)
	}
}

func HandleHTTPNotifyAmf(request *http_wrapper.Request) *http_wrapper.Response {
	// step 1: log
	logger.HttpLog.Debugln("Handle AMF HTTP Notify")

	// step 2: retrieve request
	// registerRequest := request.Body.(amfData)
	registerRequest := request.Body.(models.Amf3GppAccessRegistration)

	// step 3: handle the message
	// logger.HttpLog.Infof("registerRequest.amfNum: ", registerRequest.amfNum)
	header, response, problemDetails := MDAFProcedure(registerRequest)

	// step 4: process the return value from step 3
	if response != nil {
		// status code is based on SPEC, and option headers
		return http_wrapper.NewResponse(http.StatusCreated, header, response)
	} else if problemDetails != nil {
		return http_wrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	} else {
		return http_wrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}
