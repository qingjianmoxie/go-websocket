package define

const (
	//每组客户端数量限制
	GROUP_CLIENT_LIMIT = 500;

	//redis 分组里的客户端列表key
	REDIS_KEY_GROUP = "ws-group-client-list:"

	//redis clientId前缀
	REDIS_CLIENT_ID_PREFIX = "ws-client-id:"

	//redis 客户端ID过期时间
	REDIS_KEY_SURVIVAL_SECONDS = 172800 //2天

	//redis 分组列表key
	REDIS_KEY_GROUP_LIST = "ws-group-list"
)
