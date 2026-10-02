package constant

// SubscriptionPoolChannelTypes are the channel types whose upstream
// credentials are personal subscription accounts (OAuth) or pooled gateway
// accounts. They participate in account pool management and the
// ban-prevention policy (cooldown / isolation / per-account pacing).
var SubscriptionPoolChannelTypes = []int{
	ChannelTypeCustom,      // 8 - pooled OpenAI-compatible gateway API keys
	ChannelTypeGemini,      // 24
	ChannelTypeCodex,       // 57
	ChannelTypeSub2API,     // 59
	ChannelTypeNewAPI,      // 60
	ChannelTypeAntigravity, // 64
}

func IsSubscriptionPoolChannelType(channelType int) bool {
	for _, t := range SubscriptionPoolChannelTypes {
		if t == channelType {
			return true
		}
	}
	return false
}
