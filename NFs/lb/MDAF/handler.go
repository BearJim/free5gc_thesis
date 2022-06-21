package mdaf

import (
	"math"
	"net/http"
	"strconv"

	"loadbalance/context"
	"loadbalance/logger"

	"github.com/free5gc/openapi/models"
)

type amfInfo struct {
	amfNum  string
	ueNum   int
	cpuRate float64
}

var amf0, amf1, amf2 amfInfo

func MDAFProcedure(request models.Amf3GppAccessRegistration) (header http.Header, response *models.Amf3GppAccessRegistration, problemDetails *models.ProblemDetails) {
	logger.HttpLog.Infoln("========MDAFProcedure========")

	amfNum := request.AmfInstanceId
	ueNum := request.SupportedFeatures
	cpuRate := request.Pei
	ueNum_i, _ := strconv.Atoi(ueNum)
	cpuRate_f, _ := strconv.ParseFloat(cpuRate, 64)
	amf0.amfNum = "0"
	amf1.amfNum = "1"
	amf2.amfNum = "2"
	logger.HttpLog.Infof("amfNum: %v, ueNum: %v, cpuRate: %v", amfNum, ueNum_i, cpuRate_f)

	//update each AMF info
	switch amfNum {
	case "0":
		amf0.ueNum = ueNum_i
		amf0.cpuRate = cpuRate_f
	case "1":
		amf1.ueNum = ueNum_i
		amf1.cpuRate = cpuRate_f
	case "2":
		amf2.ueNum = ueNum_i
		amf2.cpuRate = cpuRate_f
	}
	// logger.HttpLog.Infof("amf0 ue: %v, amf1 ue: %v, amf2 ue: %v", amf0.ueNum, amf1.ueNum, amf2.ueNum)
	logger.HttpLog.Infof("amf0 cpu: %v, amf1 cpu: %v, amf2 cpu: %v", amf0.cpuRate, amf1.cpuRate, amf2.cpuRate)
	// ueNumSlice := []int{amf0.ueNum, amf1.ueNum, amf2.ueNum}
	cpuRateSlice := []float64{amf0.cpuRate, amf1.cpuRate, amf2.cpuRate}

	targetAmfp := []int{10, 0, 0} //當所有AMF都小於threshold，給AMF0
	threshold := 20.0
	maxCpu, maxCpuK := max(cpuRateSlice)
	secCpu, secCpuK := sec(cpuRateSlice)
	_, minCPuK := min(cpuRateSlice)
	if maxCpu > threshold && secCpu < threshold { //AMF數: 2
		targetAmfp[minCPuK] = 0
		if cpuRateSlice[maxCpuK] != 0 && cpuRateSlice[secCpuK] != 0 {
			u0 := 100.0 - cpuRateSlice[maxCpuK] //小
			u1 := 100.0 - cpuRateSlice[secCpuK] //大
			tr := (u0 + u1) * 0.2
			c := ((u0 + u1) - tr) / 2
			if c > u0 {
				targetAmfp[maxCpuK] = 0
				targetAmfp[secCpuK] = 10
			} else {
				targetAmfp[maxCpuK] = int(math.Floor(((u0-c)/tr)*10 + 0.5))
				targetAmfp[secCpuK] = int(math.Floor(((u1-c)/tr)*10 + 0.5))
			}
		}
	} else if maxCpu > threshold && secCpu > threshold { //AMF數: 3
		// COOP algo
		if amf0.cpuRate != 0 && amf1.cpuRate != 0 && amf2.cpuRate != 0 {
			u0 := 100.0 - amf0.cpuRate
			u1 := 100.0 - amf1.cpuRate
			u2 := 100.0 - amf2.cpuRate
			tr := (u0 + u1 + u2) * 0.2
			c := ((u0 + u1 + u2) - tr) / 3
			min, minK, sec, secK, max, maxK := compare(u0, u1, u2)
			uSlice := []float64{min, sec, max}
			uKSlice := []int{minK, secK, maxK}
			for i := 0; i < 3; i++ {
				if c > uSlice[i] && i != 2 {
					targetAmfp[uKSlice[i]] = 0
					c = (c - uSlice[i]/float64(3-i)) * float64(3-i) / float64(2-i)
				} else {
					targetAmfp[uKSlice[i]] = int(math.Floor(((uSlice[i]-c)/tr)*10 + 0.5))
				}
			}
		}
	}

	UpdateLBGoAmf(targetAmfp)

	header = make(http.Header)
	header.Set("Location", "http://127.0.0.21:8000/nmdaf-msg/v1")
	return header, &request, nil
}

func compare(u0 float64, u1 float64, u2 float64) (min float64, minK int, sec float64, secK int, max float64, maxK int) {
	if u0 >= u1 && u0 >= u2 {
		max = u0
		maxK = 0
		if u1 >= u2 {
			sec = u1
			secK = 1
			min = u2
			minK = 2
		} else {
			sec = u2
			secK = 2
			min = u1
			minK = 1
		}
	} else if u0 < u1 && u0 < u2 {
		min = u0
		minK = 0
		if u1 >= u2 {
			max = u1
			maxK = 1
			sec = u2
			secK = 2
		} else {
			max = u2
			maxK = 2
			sec = u1
			secK = 1
		}
	} else {
		sec = u0
		secK = 0
		if u1 >= u2 {
			max = u1
			maxK = 1
			min = u2
			minK = 2
		} else {
			max = u2
			maxK = 2
			min = u1
			minK = 1
		}
	}
	return
}

func max(l []float64) (max float64, maxK int) {
	max = l[0]
	for k, v := range l {
		if v > max {
			max = v
			maxK = k
		}
	}
	return
}

func min(l []float64) (min float64, minK int) {
	min = l[0]
	for k, v := range l {
		if v < min {
			min = v
			minK = k
		}
	}
	return
}

func sec(l []float64) (sec float64, secK int) {
	_, maxK := max(l)
	_, minK := min(l)
	secK = 3 - maxK - minK
	if maxK == minK { //考慮三個都一樣
		secK = 1
	}
	sec = l[secK]
	return
}

func UpdateLBGoAmf(targetAmfp []int) {
	context.LB_Self().MdafGoAmf = targetAmfp
}
