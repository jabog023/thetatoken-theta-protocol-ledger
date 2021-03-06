package dispatcher

import (
	"github.com/thetatoken/ukulele/common"
)

// MaxInventorySize defines the max number of items in InventoryRequest/InventoryResponse.
const MaxInventorySize = 500

// InventoryRequest defines the structure of the inventory request
type InventoryRequest struct {
	ChannelID common.ChannelIDEnum
	Start     string
	End       string
}

// InventoryResponse defines the structure of the inventory response
type InventoryResponse struct {
	ChannelID common.ChannelIDEnum
	Entries   []string
}

// DataRequest defines the structure of the data request
type DataRequest struct {
	ChannelID common.ChannelIDEnum
	Entries   []string
}

// DataResponse defines the structure of the data response
type DataResponse struct {
	ChannelID common.ChannelIDEnum
	Payload   common.Bytes
}
