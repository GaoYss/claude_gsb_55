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

// Meta 维修模块字典, 含历史维修人员与班组。
type Meta struct {
	Statuses    []string `json:"statuses"`
	Results     []string `json:"results"`
	Liabilities []string `json:"liabilities"`
	Natures     []string `json:"natures"`
	Repairmen   []string `json:"repairmen"`
	Teams       []string `json:"teams"`
}

// Statistics 维修统计结果。
type Statistics struct {
	Total             int64            `json:"total"`
	OngoingTotal      int64            `json:"ongoing_total"`
	FinishedTotal     int64            `json:"finished_total"`
	TotalCost         float64          `json:"total_cost"`
	AverageCost       float64          `json:"average_cost"`
	AverageDurationHr float64          `json:"average_duration_hours"`
	ByResult          map[string]int64 `json:"by_result"`
	ByLiability       map[string]int64 `json:"by_liability"`
	ByNature          map[string]int64 `json:"by_nature"`
}

// 批量更正的处理模式。
const (
	// CorrectModeAtomic 整批原子: 任一条不满足条件则整批不动(默认)。
	CorrectModeAtomic = "atomic"
	// CorrectModePartial 逐条处理: 满足条件的正常更正, 不满足的逐条返回失败原因。
	CorrectModePartial = "partial"
)

// CorrectItem 是一条更正明细, 单笔更正即长度为 1 的批量。
// RepairID 在单笔更正接口中由路径参数提供, 批量接口中必填, 服务层统一校验。
type CorrectItem struct {
	RepairID  uint   `json:"repair_id"`
	NewResult string `json:"new_result" binding:"required,oneof=fixed pending_parts observing unfixable"`
	Reason    string `json:"reason" binding:"required,max=255"`
}

// CorrectBatchRequest 批量结果更正请求。
// mode=atomic(默认): 任一条不满足条件则整批不动, 返回 409 并逐条说明原因;
// mode=partial: 逐条处理, 成功与失败分别在结果的 succeeded / failed 中返回。
type CorrectBatchRequest struct {
	Operator string        `json:"operator" binding:"required,max=64"`
	Mode     string        `json:"mode" binding:"omitempty,oneof=atomic partial"`
	Items    []CorrectItem `json:"items" binding:"required,min=1,max=100,dive"`
}

// FieldChange 是更正前后某个字段的对照。
type FieldChange struct {
	Field       string `json:"field"`
	FieldLabel  string `json:"field_label"`
	Before      string `json:"before"`
	After       string `json:"after"`
	BeforeLabel string `json:"before_label"`
	AfterLabel  string `json:"after_label"`
}

// CorrectionView 是更正记录的响应视图, 附带逐字段对照。
type CorrectionView struct {
	RepairCorrection
	Changes []FieldChange `json:"changes"`
}

// CorrectFailure 描述批量更正中单条明细的失败原因。
type CorrectFailure struct {
	Index    int    `json:"index"`
	RepairID uint   `json:"repair_id"`
	RepairNo string `json:"repair_no"`
	Message  string `json:"message"`
}

// CorrectBatchResult 是批量更正的处理结果。
type CorrectBatchResult struct {
	Mode      string           `json:"mode"`
	BatchNo   string           `json:"batch_no,omitempty"`
	Applied   int              `json:"applied"`
	Succeeded []CorrectionView `json:"succeeded"`
	Failed    []CorrectFailure `json:"failed"`
}

// CorrectionListQuery 更正记录查询条件。
type CorrectionListQuery struct {
	pagination.Params
	RepairID uint   `form:"repair_id"`
	BatchNo  string `form:"batch_no"`
	Month    string `form:"month"` // 按更正生效月份 YYYY-MM 过滤
	Operator string `form:"operator"`
}

// AggregationRow 是班组归集的一行: 班组 × 责任方 × 维修性质。
type AggregationRow struct {
	RepairTeam  string  `json:"repair_team"`
	Liability   string  `json:"liability"`
	Nature      string  `json:"nature"`
	RepairCount int64   `json:"repair_count"`
	TotalCost   float64 `json:"total_cost"`
}

// AdjustmentView 是跨月更正落入当月的调整项: 源月份已结算, 金额影响计入更正发生月。
type AdjustmentView struct {
	CorrectionID uint      `json:"correction_id"`
	BatchNo      string    `json:"batch_no"`
	RepairID     uint      `json:"repair_id"`
	RepairNo     string    `json:"repair_no"`
	RepairTeam   string    `json:"repair_team"`
	SourceMonth  string    `json:"source_month"`
	OldResult    string    `json:"old_result"`
	NewResult    string    `json:"new_result"`
	OldLiability string    `json:"old_liability"`
	NewLiability string    `json:"new_liability"`
	OldNature    string    `json:"old_nature"`
	NewNature    string    `json:"new_nature"`
	Cost         float64   `json:"cost"`
	Reason       string    `json:"reason"`
	Operator     string    `json:"operator"`
	CreatedAt    time.Time `json:"created_at"`
}

// AggregationResponse 是某月的班组归集结果。
// settled=false 时 rows 按当前生效结果实时计算(更正后同步刷新);
// settled=true 时 rows 为结算时封存的快照, 之后的跨月更正只体现在 adjustments 中。
type AggregationResponse struct {
	Month       string           `json:"month"`
	Settled     bool             `json:"settled"`
	SettledBy   string           `json:"settled_by,omitempty"`
	SettledAt   *time.Time       `json:"settled_at,omitempty"`
	Rows        []AggregationRow `json:"rows"`
	Adjustments []AdjustmentView `json:"adjustments"`
}

// SettleMonthRequest 月度结算请求, 结算后该月归集金额封存。
type SettleMonthRequest struct {
	Month     string `json:"month" binding:"required,len=7"`
	SettledBy string `json:"settled_by" binding:"required,max=64"`
	Note      string `json:"note" binding:"omitempty,max=255"`
}
