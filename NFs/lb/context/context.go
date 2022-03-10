package context

import (
	"fmt"
	"math"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"loadbalance/factory"

	"loadbalance/logger"

	"github.com/free5gc/idgenerator"
	"github.com/free5gc/openapi/models"
)

var (
	lbContext                                                = LBContext{}
	tmsiGenerator                   *idgenerator.IDGenerator = nil
	lbUeNGAPIDGenerator             *idgenerator.IDGenerator = nil
	lbStatusSubscriptionIDGenerator *idgenerator.IDGenerator = nil
)

func init() {
	LB_Self().LadnPool = make(map[string]*LADN)
	LB_Self().EventSubscriptionIDGenerator = idgenerator.NewGenerator(1, math.MaxInt32)
	LB_Self().Name = "lb"
	LB_Self().UriScheme = models.UriScheme_HTTPS
	LB_Self().RelativeCapacity = 0xff
	LB_Self().ServedGuamiList = make([]models.Guami, 0, MaxNumOfServedGuamiList)
	LB_Self().PlmnSupportList = make([]factory.PlmnSupportItem, 0, MaxNumOfPLMNs)
	LB_Self().NfService = make(map[models.ServiceName]models.NfService)
	LB_Self().NetworkName.Full = "free5GC"
	tmsiGenerator = idgenerator.NewGenerator(1, math.MaxInt32)
	lbStatusSubscriptionIDGenerator = idgenerator.NewGenerator(1, math.MaxInt32)
	lbUeNGAPIDGenerator = idgenerator.NewGenerator(1, MaxValueOfLbUeNgapId)
}

type LBContext struct {
	EventSubscriptionIDGenerator    *idgenerator.IDGenerator
	EventSubscriptions              sync.Map
	UePool                          sync.Map         // map[supi]*LbUe
	RanUePool                       sync.Map         // map[LbUeNgapID]*RanUe
	LbRanPool                       sync.Map         // map[net.Conn]*LbRan
	LadnPool                        map[string]*LADN // dnn as key
	SupportTaiLists                 []models.Tai
	ServedGuamiList                 []models.Guami
	PlmnSupportList                 []factory.PlmnSupportItem
	RelativeCapacity                int64
	NfId                            string
	Name                            string
	NfService                       map[models.ServiceName]models.NfService // nfservice that lb support
	UriScheme                       models.UriScheme
	BindingIPv4                     string
	SBIPort                         int
	RegisterIPv4                    string
	HttpIPv6Address                 string
	TNLWeightFactor                 int64
	SupportDnnLists                 []string
	LBStatusSubscriptions           sync.Map // map[subscriptionID]models.SubscriptionData
	NrfUri                          string
	SecurityAlgorithm               SecurityAlgorithm
	NetworkName                     factory.NetworkName
	NgapIpList                      []string // NGAP Server IP
	T3502Value                      int      // unit is second
	T3512Value                      int      // unit is second
	Non3gppDeregistrationTimerValue int      // unit is second
	// read-only fields
	T3513Cfg factory.TimerValue
	T3522Cfg factory.TimerValue
	T3550Cfg factory.TimerValue
	T3560Cfg factory.TimerValue
	T3565Cfg factory.TimerValue
	Locality string
}

type LBContextEventSubscription struct {
	IsAnyUe           bool
	IsGroupUe         bool
	UeSupiList        []string
	Expiry            *time.Time
	EventSubscription models.AmfEventSubscription
}

type SecurityAlgorithm struct {
	IntegrityOrder []uint8 // slice of security.AlgIntegrityXXX
	CipheringOrder []uint8 // slice of security.AlgCipheringXXX
}

func NewPlmnSupportItem() (item factory.PlmnSupportItem) {
	item.SNssaiList = make([]models.Snssai, 0, MaxNumOfSlice)
	return
}

func (context *LBContext) TmsiAllocate() int32 {
	tmsi, err := tmsiGenerator.Allocate()
	if err != nil {
		logger.ContextLog.Errorf("Allocate TMSI error: %+v", err)
		return -1
	}
	return int32(tmsi)
}

func (context *LBContext) AllocateLbUeNgapID() (int64, error) {
	return lbUeNGAPIDGenerator.Allocate()
}

func (context *LBContext) AllocateGutiToUe(ue *LbUe) {
	servedGuami := context.ServedGuamiList[0]
	ue.Tmsi = context.TmsiAllocate()

	plmnID := servedGuami.PlmnId.Mcc + servedGuami.PlmnId.Mnc
	tmsiStr := fmt.Sprintf("%08x", ue.Tmsi)
	ue.Guti = plmnID + servedGuami.AmfId + tmsiStr
}

func (context *LBContext) AllocateRegistrationArea(ue *LbUe, anType models.AccessType) {
	// clear the previous registration area if need
	if len(ue.RegistrationArea[anType]) > 0 {
		ue.RegistrationArea[anType] = nil
	}

	// allocate a new tai list as a registration area to ue
	// TODO: algorithm to choose TAI list
	for _, supportTai := range context.SupportTaiLists {
		if reflect.DeepEqual(supportTai, ue.Tai) {
			ue.RegistrationArea[anType] = append(ue.RegistrationArea[anType], supportTai)
			break
		}
	}
}

func (context *LBContext) NewLBStatusSubscription(subscriptionData models.SubscriptionData) (subscriptionID string) {
	id, err := lbStatusSubscriptionIDGenerator.Allocate()
	if err != nil {
		logger.ContextLog.Errorf("Allocate subscriptionID error: %+v", err)
		return ""
	}

	subscriptionID = strconv.Itoa(int(id))
	context.LBStatusSubscriptions.Store(subscriptionID, subscriptionData)
	return
}

// Return Value: (subscriptionData *models.SubScriptionData, ok bool)
func (context *LBContext) FindLBStatusSubscription(subscriptionID string) (*models.SubscriptionData, bool) {
	if value, ok := context.LBStatusSubscriptions.Load(subscriptionID); ok {
		subscriptionData := value.(models.SubscriptionData)
		return &subscriptionData, ok
	} else {
		return nil, false
	}
}

func (context *LBContext) DeleteLBStatusSubscription(subscriptionID string) {
	context.LBStatusSubscriptions.Delete(subscriptionID)
	if id, err := strconv.ParseInt(subscriptionID, 10, 64); err != nil {
		logger.ContextLog.Error(err)
	} else {
		lbStatusSubscriptionIDGenerator.FreeID(id)
	}
}

func (context *LBContext) NewEventSubscription(subscriptionID string, subscription *LBContextEventSubscription) {
	context.EventSubscriptions.Store(subscriptionID, subscription)
}

func (context *LBContext) FindEventSubscription(subscriptionID string) (*LBContextEventSubscription, bool) {
	if value, ok := context.EventSubscriptions.Load(subscriptionID); ok {
		return value.(*LBContextEventSubscription), ok
	} else {
		return nil, false
	}
}

func (context *LBContext) DeleteEventSubscription(subscriptionID string) {
	context.EventSubscriptions.Delete(subscriptionID)
	if id, err := strconv.ParseInt(subscriptionID, 10, 32); err != nil {
		logger.ContextLog.Error(err)
	} else {
		context.EventSubscriptionIDGenerator.FreeID(id)
	}
}

func (context *LBContext) AddLbUeToUePool(ue *LbUe, supi string) {
	if len(supi) == 0 {
		logger.ContextLog.Errorf("Supi is nil")
	}
	ue.Supi = supi
	context.UePool.Store(ue.Supi, ue)
}

func (context *LBContext) NewLbUe(supi string) *LbUe {
	ue := LbUe{}
	ue.init()

	if supi != "" {
		context.AddLbUeToUePool(&ue, supi)
	}

	context.AllocateGutiToUe(&ue)

	return &ue
}

func (context *LBContext) LbUeFindByUeContextID(ueContextID string) (*LbUe, bool) {
	if strings.HasPrefix(ueContextID, "imsi") {
		return context.LbUeFindBySupi(ueContextID)
	}
	if strings.HasPrefix(ueContextID, "imei") {
		return context.LbUeFindByPei(ueContextID)
	}
	if strings.HasPrefix(ueContextID, "5g-guti") {
		guti := ueContextID[strings.LastIndex(ueContextID, "-")+1:]
		return context.LbUeFindByGuti(guti)
	}
	return nil, false
}

func (context *LBContext) LbUeFindBySupi(supi string) (ue *LbUe, ok bool) {
	if value, loadOk := context.UePool.Load(supi); loadOk {
		ue = value.(*LbUe)
		ok = loadOk
	}
	return
}

func (context *LBContext) LbUeFindByPei(pei string) (ue *LbUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*LbUe)
		if ok = (candidate.Pei == pei); ok {
			ue = candidate
			return false
		}
		return true
	})
	return
}

func (context *LBContext) NewLbRan(conn net.Conn) *LbRan {
	ran := LbRan{}
	ran.SupportedTAList = make([]SupportedTAI, 0, MaxNumOfTAI*MaxNumOfBroadcastPLMNs)
	ran.Conn = conn
	ran.Log = logger.NgapLog.WithField(logger.FieldRanAddr, conn.RemoteAddr().String())
	context.LbRanPool.Store(conn, &ran)
	return &ran
}

// use net.Conn to find RAN context, return *LbRan and ok bit
func (context *LBContext) LbRanFindByConn(conn net.Conn) (*LbRan, bool) {
	if value, ok := context.LbRanPool.Load(conn); ok {
		return value.(*LbRan), ok
	}
	return nil, false
}

// use ranNodeID to find RAN context, return *LbRan and ok bit
func (context *LBContext) LbRanFindByRanID(ranNodeID models.GlobalRanNodeId) (*LbRan, bool) {
	var ran *LbRan
	var ok bool
	context.LbRanPool.Range(func(key, value interface{}) bool {
		lbRan := value.(*LbRan)
		switch lbRan.RanPresent {
		case RanPresentGNbId:
			if lbRan.RanId.GNbId.GNBValue == ranNodeID.GNbId.GNBValue {
				ran = lbRan
				ok = true
				return false
			}
		case RanPresentNgeNbId:
			if lbRan.RanId.NgeNbId == ranNodeID.NgeNbId {
				ran = lbRan
				ok = true
				return false
			}
		case RanPresentN3IwfId:
			if lbRan.RanId.N3IwfId == ranNodeID.N3IwfId {
				ran = lbRan
				ok = true
				return false
			}
		}
		return true
	})
	return ran, ok
}

func (context *LBContext) DeleteLbRan(conn net.Conn) {
	context.LbRanPool.Delete(conn)
}

func (context *LBContext) InSupportDnnList(targetDnn string) bool {
	for _, dnn := range context.SupportDnnLists {
		if dnn == targetDnn {
			return true
		}
	}
	return false
}

func (context *LBContext) InPlmnSupportList(snssai models.Snssai) bool {
	for _, plmnSupportItem := range context.PlmnSupportList {
		for _, supportSnssai := range plmnSupportItem.SNssaiList {
			if reflect.DeepEqual(supportSnssai, snssai) {
				return true
			}
		}
	}
	return false
}

func (context *LBContext) LbUeFindByGuti(guti string) (ue *LbUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*LbUe)
		if ok = (candidate.Guti == guti); ok {
			ue = candidate
			return false
		}
		return true
	})
	return
}

func (context *LBContext) LbUeFindByPolicyAssociationID(polAssoId string) (ue *LbUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*LbUe)
		if ok = (candidate.PolicyAssociationId == polAssoId); ok {
			ue = candidate
			return false
		}
		return true
	})
	return
}

func (context *LBContext) RanUeFindByLbUeNgapID(lbUeNgapID int64) *RanUe {
	if value, ok := context.RanUePool.Load(lbUeNgapID); ok {
		return value.(*RanUe)
	} else {
		return nil
	}
}

func (context *LBContext) GetIPv4Uri() string {
	return fmt.Sprintf("%s://%s:%d", context.UriScheme, context.RegisterIPv4, context.SBIPort)
}

func (context *LBContext) InitNFService(serivceName []string, version string) {
	tmpVersion := strings.Split(version, ".")
	versionUri := "v" + tmpVersion[0]
	for index, nameString := range serivceName {
		name := models.ServiceName(nameString)
		context.NfService[name] = models.NfService{
			ServiceInstanceId: strconv.Itoa(index),
			ServiceName:       name,
			Versions: &[]models.NfServiceVersion{
				{
					ApiFullVersion:  version,
					ApiVersionInUri: versionUri,
				},
			},
			Scheme:          context.UriScheme,
			NfServiceStatus: models.NfServiceStatus_REGISTERED,
			ApiPrefix:       context.GetIPv4Uri(),
			IpEndPoints: &[]models.IpEndPoint{
				{
					Ipv4Address: context.RegisterIPv4,
					Transport:   models.TransportProtocol_TCP,
					Port:        int32(context.SBIPort),
				},
			},
		}
	}
}

// Reset LB Context
func (context *LBContext) Reset() {
	context.LbRanPool.Range(func(key, value interface{}) bool {
		context.UePool.Delete(key)
		return true
	})
	for key := range context.LadnPool {
		delete(context.LadnPool, key)
	}
	context.RanUePool.Range(func(key, value interface{}) bool {
		context.RanUePool.Delete(key)
		return true
	})
	context.UePool.Range(func(key, value interface{}) bool {
		context.UePool.Delete(key)
		return true
	})
	context.EventSubscriptions.Range(func(key, value interface{}) bool {
		context.DeleteEventSubscription(key.(string))
		return true
	})
	for key := range context.NfService {
		delete(context.NfService, key)
	}
	context.SupportTaiLists = context.SupportTaiLists[:0]
	context.PlmnSupportList = context.PlmnSupportList[:0]
	context.ServedGuamiList = context.ServedGuamiList[:0]
	context.RelativeCapacity = 0xff
	context.NfId = ""
	context.UriScheme = models.UriScheme_HTTPS
	context.SBIPort = 0
	context.BindingIPv4 = ""
	context.RegisterIPv4 = ""
	context.HttpIPv6Address = ""
	context.Name = "lb"
	context.NrfUri = ""
}

// Create new LB context
func LB_Self() *LBContext {
	return &lbContext
}
