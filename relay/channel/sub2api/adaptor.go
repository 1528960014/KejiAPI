package sub2api

import (
	"kejiapi/relay/channel/kejiapi"
)

type Adaptor struct {
	kejiapi.Adaptor
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
