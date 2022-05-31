package consumer

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	amf_context "github.com/free5gc/amf1/context"
	"github.com/free5gc/amf1/logger"
	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	"github.com/shirou/gopsutil/cpu"
	"golang.org/x/net/http2"
)

type Configuration struct {
	url           string
	basePath      string
	host          string
	defaultHeader map[string]string
	userAgent     string
	httpClient    *http.Client
}

type APIClient struct {
	cfg           *Configuration
	common        service // Reuse a single struct instead of allocating one for each service on the heap.
	AMFMdafMsgApi *AMFMdafMsgService
}

type service struct {
	client *APIClient
}

// type amfData struct {
// 	amfNum  string `json:"amfId" bson:"amfId"`
// 	ueNum   string `json:"ueNum" bson:"UeNum"`
// 	cpuRate string `json:"cpuRate" bson:"cpuRate"`
// }

type AMFMdafMsgService service

func MdafMsg() (*models.ProblemDetails, error) {
	logger.HttpLog.Infoln("Start MdafMsg")
	configuration := NewConfiguration()
	configuration.SetBasePath("http://127.0.0.21:8000") //TH LB http IP
	client := NewAPIClient(configuration)
	amfSelf := amf_context.AMF_Self()

	tempUeNum := amfSelf.UeNum
	ueNum := strconv.Itoa(tempUeNum)

	tempCpuRate, err := getCpuInfo()
	if err != nil {
		return nil, err
	}
	// totalCpuRate := strconv.Itoa(tempCpuRate[0])
	totalCpuRate := strconv.FormatFloat(tempCpuRate[0], 'f', 3, 64)
	// amfDataBody := amfData{
	// 	amfNum:  0,
	// 	ueNum:   tempUeNum,
	// 	cpuRate: tempCpuRate[0],
	// }

	registrationData := models.Amf3GppAccessRegistration{
		AmfInstanceId:     "0",
		SupportedFeatures: ueNum,
		Pei:               totalCpuRate,
	}

	logger.HttpLog.Infoln("===Start AmfMdafMsg===")
	// logger.HttpLog.Infof("amfNum: %v ueNum: %v cpuRate: %v", amfDataBody.amfNum, amfDataBody.ueNum, amfDataBody.cpuRate)
	// _, httpResp, localErr := client.AMFMdafMsgApi.AmfMdafMsg(context.Background(), amfDataBody)
	_, httpResp, localErr := client.AMFMdafMsgApi.AmfMdafMsg(context.Background(), registrationData)
	if localErr == nil {
		return nil, nil
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			return nil, localErr
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		return &problem, nil
	} else {
		return nil, openapi.ReportError("server no response")
	}
}

func NewConfiguration() *Configuration {
	cfg := &Configuration{
		basePath:      "https://example.com/nmdaf-msg/v1",
		url:           "{apiRoot}/nmdaf-msg/v1",
		defaultHeader: make(map[string]string),
		userAgent:     "OpenAPI-Generator/1.0.0/go",
	}
	return cfg
}

func (c *Configuration) SetBasePath(apiRoot string) {
	url := c.url

	// Replace apiRoot
	url = strings.Replace(url, "{"+"apiRoot"+"}", apiRoot, -1)

	c.basePath = url
}

func (c *Configuration) BasePath() string {
	return c.basePath
}

func (c *Configuration) Host() string {
	return c.host
}

func (c *Configuration) SetHost(host string) {
	c.host = host
}

func (c *Configuration) UserAgent() string {
	return c.userAgent
}

func (c *Configuration) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

func (c *Configuration) DefaultHeader() map[string]string {
	return c.defaultHeader
}

func (c *Configuration) AddDefaultHeader(key string, value string) {
	c.defaultHeader[key] = value
}

func (c *Configuration) HTTPClient() *http.Client {
	return c.httpClient
}

func NewAPIClient(cfg *Configuration) *APIClient {
	if cfg.httpClient == nil {
		cfg.httpClient = http.DefaultClient
		cfg.httpClient.Transport = &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	c := &APIClient{}
	c.cfg = cfg
	c.common.client = c

	// API Services
	c.AMFMdafMsgApi = (*AMFMdafMsgService)(&c.common)

	return c
}

// func (a *AMFMdafMsgService) AmfMdafMsg(ctx context.Context, tempamfData amfData) (amfData, *http.Response, error) {
func (a *AMFMdafMsgService) AmfMdafMsg(ctx context.Context, registration models.Amf3GppAccessRegistration) (models.Amf3GppAccessRegistration, *http.Response, error) {
	logger.HttpLog.Infoln("===In AmfMdafMsg===")
	var (
		localVarHTTPMethod   = strings.ToUpper("Put")
		localVarPostBody     interface{}
		localVarFormFileName string
		localVarFileName     string
		localVarFileBytes    []byte
		localVarReturnValue  models.Amf3GppAccessRegistration
	)

	// create path and map variables
	localVarPath := a.client.cfg.BasePath() + "/notify"

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	localVarHTTPContentTypes := []string{"application/json"}

	// use the first content type specified in 'consumes'
	localVarHeaderParams["Content-Type"] = localVarHTTPContentTypes[0]

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json", "application/problem+json"}

	// set Accept header
	localVarHTTPHeaderAccept := openapi.SelectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}

	// body params
	localVarPostBody = &registration
	logger.HttpLog.Infoln("localVarPostBody: ", localVarPostBody)

	r, err := openapi.PrepareRequest(ctx, a.client.cfg, localVarPath, localVarHTTPMethod,
		localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams,
		localVarFormFileName, localVarFileName, localVarFileBytes)
	if err != nil {
		logger.HttpLog.Errorln("===PrepareRequest===")
		return localVarReturnValue, nil, err
	}

	logger.HttpLog.Infoln("PrepareRequest: ", r.GetBody)

	localVarHTTPResponse, err := openapi.CallAPI(a.client.cfg, r)
	if err != nil || localVarHTTPResponse == nil {
		logger.HttpLog.Errorln("===CallAPI===")
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := ioutil.ReadAll(localVarHTTPResponse.Body)
	// err = localVarHTTPResponse.Body.Close()
	if err != nil {
		logger.HttpLog.Errorln("===ReadAll===")
		return localVarReturnValue, localVarHTTPResponse, err
	}

	apiError := openapi.GenericOpenAPIError{
		RawBody:     localVarBody,
		ErrorStatus: localVarHTTPResponse.Status,
	}

	switch localVarHTTPResponse.StatusCode {
	case 201:
		err = openapi.Deserialize(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
		}
		return localVarReturnValue, localVarHTTPResponse, nil
	case 200:
		err = openapi.Deserialize(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
		}
		return localVarReturnValue, localVarHTTPResponse, nil
	case 204:
		return localVarReturnValue, localVarHTTPResponse, nil
	case 400:
		var v models.ProblemDetails
		err = openapi.Deserialize(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
			return localVarReturnValue, localVarHTTPResponse, apiError
		}
		apiError.ErrorModel = v
		return localVarReturnValue, localVarHTTPResponse, apiError
	case 403:
		var v models.ProblemDetails
		err = openapi.Deserialize(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
			return localVarReturnValue, localVarHTTPResponse, apiError
		}
		apiError.ErrorModel = v
		return localVarReturnValue, localVarHTTPResponse, apiError
	case 404:
		var v models.ProblemDetails
		err = openapi.Deserialize(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
			return localVarReturnValue, localVarHTTPResponse, apiError
		}
		apiError.ErrorModel = v
		return localVarReturnValue, localVarHTTPResponse, apiError
	case 500:
		var v models.ProblemDetails
		err = openapi.Deserialize(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
			return localVarReturnValue, localVarHTTPResponse, apiError
		}
		apiError.ErrorModel = v
		return localVarReturnValue, localVarHTTPResponse, apiError
	case 503:
		var v models.ProblemDetails
		err = openapi.Deserialize(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
		if err != nil {
			apiError.ErrorStatus = err.Error()
			return localVarReturnValue, localVarHTTPResponse, apiError
		}
		apiError.ErrorModel = v
		return localVarReturnValue, localVarHTTPResponse, apiError
	default:
		return localVarReturnValue, localVarHTTPResponse, nil
	}
}

func getCpuInfo() ([]float64, error) {
	totalPercent, err := cpu.Percent(3*time.Second, false)
	if err != nil {
		return nil, err
	}
	return totalPercent, nil
}
