package dto

// UserDashboardBrief 用户数据看板的轻量用户信息（避免前端二次请求）
type UserDashboardBrief struct {
	Id           int    `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Role         int    `json:"role"`
	Group        string `json:"group"`
	Quota        int    `json:"quota"`
	UsedQuota    int    `json:"used_quota"`
	RequestCount int    `json:"request_count"`
}

// UserDashboardPayload admin 数据看板接口返回结构。
// Dates 字段承载 []*model.QuotaData 序列化结果；此处使用 any 以避免 model→dto→model 的循环导入。
type UserDashboardPayload struct {
	User  UserDashboardBrief `json:"user"`
	Dates any                `json:"dates"`
}
