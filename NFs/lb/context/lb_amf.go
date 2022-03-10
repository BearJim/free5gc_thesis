package context

import (
	"net"

	"github.com/sirupsen/logrus"

	"github.com/free5gc/openapi/models"
)

type LbAmf struct {
	RanPresent int
	RanId      *models.GlobalRanNodeId
	Name       string
	AnType     models.AccessType
	/* socket Connect*/
	Conn net.Conn
	/* Supported TA List */
	SupportedTAList []SupportedTAI

	/* RAN UE List */
	RanUeList []*RanUe // RanUeNgapId as key

	/* logger */
	Log *logrus.Entry
}
