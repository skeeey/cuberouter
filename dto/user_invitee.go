package dto

// InviteeBrief 用户邀请关系列表项（最小化字段集合，无 phone_region —— 本仓库无此列）
type InviteeBrief struct {
	Id        int    `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Status    int    `json:"status"`
	Group     string `json:"group"`
	Role      int    `json:"role"`
	CreatedAt int64  `json:"created_at"`
}
