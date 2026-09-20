package repair

import (
	"time"

	"streetlight/pkg/pagination"
)

// CreateRequest 维修记录录入请求。
type CreateRequest struct {
	FaultID      uint     `json:"fault_id" binding:"required"`
	Repairman    string   `json:"repairman" binding:"required,max=64"`
	RepairTeam   string   `json:"repair_team" binding:"max=64"`
	ContactPhone string   `json:"contact_phone" binding:"max=32"`
	StartedAt    string   `json:"started_at" binding:"omitempty,max=32"`
	Content      string   `json:"content" binding:"max=512"`
	Materials    string   `json:"materials" binding:"max=255"`
	Cost         *float64 `json:"cost" binding:"omitempty,min=0"`
	Remark       string   `json:"remark" binding:"max=255"`
}

// UpdateRequest 修改维修记录, 仅未完成的记录允许修改。
type UpdateRequest struct {
	Repairman    *string  `json:"repairman" binding:"omitempty,max=64"`
	RepairTeam   *string  `json:"repair_team" binding:"omitempty,max=64"`
	ContactPhone *string  `json:"contact_phone" binding:"omitempty,max=32"`
	StartedAt    *string  `json:"started_at" binding:"omitempty,max=32"`
	Content      *string  `json:"content" binding:"omitempty,max=512"`
	Materials    *string  `json:"materials" binding:"omitempty,max=255"`
	Cost         *float64 `json:"cost" binding:"omitempty,min=0"`
	Remark       *string  `json:"remark" binding:"omitempty,max=255"`
}

// FinishRequest 完成维修请求, 提交后维修记录闭环并联动故障状态。
type FinishRequest struct {
	FinishedAt string   `json:"finished_at" binding:"omitempty,max=32"`
	Result     string   `json:"result" binding:"required,oneof=fixed pending_parts observing unfixable"`
	Content    string   `json:"content" binding:"omitempty,max=512"`
	Materials  string   `json:"materials" binding:"omitempty,max=255"`
	Cost       *float64 `json:"cost" binding:"omitempty,min=0"`
	Remark     string   `json:"remark" binding:"omitempty,max=255"`
}

// ListQuery 维修记录查询条件。
type ListQuery struct {
	pagination.Params
	Keyword    string `form:"keyword"` // 维修单号 / 故障单号 / 路灯编号 / 维修人员
	FaultID    uint   `form:"fault_id"`
	LampID     uint   `form:"lamp_id"`
	Repairman  string `form:"repairman"`
	RepairTeam string `form:"repair_team"`
	Status     string `form:"status"`
	Result     string `form:"result"`
	StartDate  string `form:"start_date"`
	EndDate    string `form:"end_date"`
}

// CorrectItem 是单条更正内容, result 与 cost 至少提供一项。
type CorrectItem struct {
	RepairID uint     `json:"repair_id" binding:"required"`
	Result   *string  `json:"result" binding:"omitempty,oneof=fixed pending_parts observing unfixable"`
	Cost     *float64 `json:"cost" binding:"omitempty,min=0"`
}

// CorrectOneRequest 单条更正请求, 与批量更正共用同一套校验与落库逻辑。
type CorrectOneRequest struct {
	Result   *string  `json:"result" binding:"omitempty,oneof=fixed pending_parts observing unfixable"`
	Cost     *float64 `json:"cost" binding:"omitempty,min=0"`
	Reason   string   `json:"reason" binding:"required,max=255"`
	Operator string   `json:"operator" binding:"required,max=64"`
}

// CorrectRequest 是批量更正请求, 单条更正等价于长度为 1 的批次。
// 批量语义: 整批原子 —— 任一条目不满足条件时整批不执行,
// 响应以 409 返回并逐条列出失败明细。
type CorrectRequest struct {
	Items    []CorrectItem `json:"items" binding:"required,min=1,max=100,dive"`
	Reason   string        `json:"reason" binding:"required,max=255"`
	Operator string        `json:"operator" binding:"required,max=64"`
}

// CorrectItemError 是批量更正中单条目的校验失败明细。
type CorrectItemError struct {
	RepairID uint   `json:"repair_id"`
	RepairNo string `json:"repair_no,omitempty"`
	Message  string `json:"message"`
}

// BatchCorrectError 表示批量更正校验失败, 整批未执行。
type BatchCorrectError struct {
	Items []CorrectItemError
}

// Error 实现 error 接口。
func (e *BatchCorrectError) Error() string {
	return "批量更正校验失败, 整批未执行"
}

// CorrectResult 是更正成功后的响应。
type CorrectResult struct {
	BatchNo   string           `json:"batch_no"`
	Revisions []RepairRevision `json:"revisions"`
}

// RevisionListQuery 修订记录查询条件。
type RevisionListQuery struct {
	pagination.Params
	RepairID uint   `form:"repair_id"`
	BatchNo  string `form:"batch_no"`
	Month    string `form:"month"` // 归集生效月 YYYY-MM
}

// AggregationQuery 班组月度归集查询条件。
type AggregationQuery struct {
	Month string `form:"month"` // YYYY-MM, 默认当前月
}

// ResultBucketView 是按结果分组的归集视图, 附带责任方与维修性质。
type ResultBucketView struct {
	Result       string  `json:"result"`
	ResultLabel  string  `json:"result_label"`
	LiableParty  string  `json:"liable_party"`
	RepairNature string  `json:"repair_nature"`
	Count        int64   `json:"count"`
	Cost         float64 `json:"cost"`
}

// TeamAggregation 是一个班组的月度归集视图。
type TeamAggregation struct {
	RepairTeam string             `json:"repair_team"`
	TotalCount int64              `json:"total_count"`
	TotalCost  float64            `json:"total_cost"`
	ByResult   []ResultBucketView `json:"by_result"`
}

// AdjustmentView 是跨已结算月更正形成的调整项, 计入生效月而不改写历史月。
type AdjustmentView struct {
	RevisionNo   string  `json:"revision_no"`
	RepairNo     string  `json:"repair_no"`
	RepairTeam   string  `json:"repair_team"`
	OriginMonth  string  `json:"origin_month"`
	OldResult    string  `json:"old_result"`
	NewResult    string  `json:"new_result"`
	CostDelta    float64 `json:"cost_delta"`
	Reason       string  `json:"reason"`
	Operator     string  `json:"operator"`
	CreatedAt    string  `json:"created_at"`
}

// MonthAggregation 是某月的班组归集总览: 未结算月实时计算, 已结算月返回冻结快照。
type MonthAggregation struct {
	Month       string            `json:"month"`
	Settled     bool              `json:"settled"`
	SettledBy   string            `json:"settled_by,omitempty"`
	SettledAt   *time.Time        `json:"settled_at,omitempty"`
	Teams       []TeamAggregation `json:"teams"`
	Adjustments []AdjustmentView  `json:"adjustments"`
}

// SettleRequest 月度结算请求。
type SettleRequest struct {
	Month    string `json:"month" binding:"required,len=7"`
	Operator string `json:"operator" binding:"required,max=64"`
}

// Meta 维修模块字典, 含历史维修人员与班组。
type Meta struct {
	Statuses    []string     `json:"statuses"`
	Results     []string     `json:"results"`
	ResultMetas []ResultMeta `json:"result_metas"`
	Repairmen   []string     `json:"repairmen"`
	Teams       []string     `json:"teams"`
}

// Statistics 维修统计结果。
type Statistics struct {
	Total             int64              `json:"total"`
	OngoingTotal      int64              `json:"ongoing_total"`
	FinishedTotal     int64              `json:"finished_total"`
	TotalCost         float64            `json:"total_cost"`
	AverageCost       float64            `json:"average_cost"`
	AverageDurationHr float64            `json:"average_duration_hours"`
	ByResult          []ResultBucketView `json:"by_result"`
}
