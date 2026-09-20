package repair

import "time"

// 维修记录状态。
const (
	StatusOngoing  = "ongoing"  // 维修中
	StatusFinished = "finished" // 已完成
)

// 维修结果。
const (
	ResultFixed        = "fixed"         // 已修复
	ResultPendingParts = "pending_parts" // 待配件
	ResultObserving    = "observing"     // 观察中
	ResultUnfixable    = "unfixable"     // 无法修复
)

// Statuses 返回全部维修记录状态。
func Statuses() []string {
	return []string{StatusOngoing, StatusFinished}
}

// Results 返回全部维修结果取值。
func Results() []string {
	return []string{ResultFixed, ResultPendingParts, ResultObserving, ResultUnfixable}
}

// IsValidResult 校验维修结果取值。
func IsValidResult(result string) bool {
	for _, item := range Results() {
		if item == result {
			return true
		}
	}
	return false
}

// Repair 维修记录, 一条记录对应故障的一次维修过程。
//
// 完工时写入的 Result / FinishedAt / Cost 是历史留痕, 完工后不再改写;
// 结果更正通过 RepairRevision 修订记录进行, 统计与归集口径读 CurrentResult / CurrentCost。
type Repair struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	RepairNo     string     `gorm:"size:64;uniqueIndex;not null" json:"repair_no"`
	FaultID      uint       `gorm:"index;not null" json:"fault_id"`
	FaultNo      string     `gorm:"size:64;index" json:"fault_no"`
	LampID       uint       `gorm:"index" json:"lamp_id"`
	LampCode     string     `gorm:"size:64;index" json:"lamp_code"`
	Repairman    string     `gorm:"size:64;index;not null" json:"repairman"`
	RepairTeam   string     `gorm:"size:64;index" json:"repair_team"`
	ContactPhone string     `gorm:"size:32" json:"contact_phone"`
	StartedAt    time.Time  `gorm:"index;not null" json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	Status       string     `gorm:"size:32;index;not null;default:ongoing" json:"status"`
	Result       string     `gorm:"size:32;index" json:"result"`
	Content      string     `gorm:"size:512" json:"content"`
	Materials    string     `gorm:"size:255" json:"materials"`
	Cost         float64    `json:"cost"`
	Remark       string     `gorm:"size:255" json:"remark"`

	// 生效值: 完工时与 Result / Cost 一致, 结果更正后指向最新取值。
	CurrentResult string  `gorm:"size:32;index" json:"current_result"`
	CurrentCost   float64 `json:"current_cost"`
	// RevisionCount 是已发生的更正次数, 用于列表标记"已更正"。
	RevisionCount int `gorm:"not null;default:0" json:"revision_count"`

	// DurationMinutes 仅用于响应展示的维修耗时(分钟), 不落库。
	DurationMinutes *int64 `gorm:"-" json:"duration_minutes,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Repair) TableName() string { return "repair" }

// FillDuration 依据开工与完工时间计算维修耗时。
func (r *Repair) FillDuration() {
	if r.FinishedAt == nil {
		r.DurationMinutes = nil
		return
	}
	minutes := int64(r.FinishedAt.Sub(r.StartedAt).Minutes())
	if minutes < 0 {
		minutes = 0
	}
	r.DurationMinutes = &minutes
}

// EffectiveResult 返回当前生效的维修结果(统计口径); 未完工或历史数据未回填时回退原始结果。
func (r *Repair) EffectiveResult() string {
	if r.CurrentResult != "" {
		return r.CurrentResult
	}
	return r.Result
}

// EffectiveCost 返回当前生效的维修费用(归集口径); 未完工记录返回草稿费用。
func (r *Repair) EffectiveCost() float64 {
	if r.Status == StatusFinished {
		return r.CurrentCost
	}
	return r.Cost
}

// FieldChange 记录一次更正里单个字段的前后对照。
type FieldChange struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

// RepairRevision 维修结果修订记录, 一次更正产生一条, 原维修记录不被改写。
// 跨已结算月的更正不改动历史归集, 以调整项计入 EffectiveMonth。
type RepairRevision struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	RevisionNo string `gorm:"size:64;uniqueIndex;not null" json:"revision_no"`
	RepairID   uint   `gorm:"index;not null" json:"repair_id"`
	RepairNo   string `gorm:"size:64;index" json:"repair_no"`
	FaultID    uint   `gorm:"index" json:"fault_id"`
	RepairTeam string `gorm:"size:64;index" json:"repair_team"`
	// BatchNo 是同一批更正共享的批次号, 单条更正也独占一个批次。
	BatchNo string `gorm:"size:64;index" json:"batch_no"`
	// Seq 是该维修记录的第几次修订, 从 1 开始递增。
	Seq       int     `gorm:"not null" json:"seq"`
	OldResult string  `gorm:"size:32" json:"old_result"`
	NewResult string  `gorm:"size:32" json:"new_result"`
	OldCost   float64 `json:"old_cost"`
	NewCost   float64 `json:"new_cost"`
	// Changes 是逐字段对照明细, 序列化为 JSON 存储。
	Changes  []FieldChange `gorm:"serializer:json" json:"changes"`
	Reason   string        `gorm:"size:255;not null" json:"reason"`
	Operator string        `gorm:"size:64;not null" json:"operator"`
	// OriginMonth 是原完工所在月(YYYY-MM), EffectiveMonth 是本次更正归集的生效月。
	OriginMonth    string `gorm:"size:7;index" json:"origin_month"`
	EffectiveMonth string `gorm:"size:7;index" json:"effective_month"`
	// CrossSettled 为 true 表示原完工月已结算, 本次更正以调整项计入生效月。
	CrossSettled bool `gorm:"index" json:"cross_settled"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名。
func (RepairRevision) TableName() string { return "repair_revision" }

// ResultBucket 是按维修结果分组的数量与金额。
type ResultBucket struct {
	Result string  `json:"result"`
	Count  int64   `json:"count"`
	Cost   float64 `json:"cost"`
}

// TeamMonthSettlement 班组月度归集结算快照, 落库后该月归集金额冻结,
// 之后的跨月更正只能以调整项计入后续开放月份。
type TeamMonthSettlement struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	Month      string `gorm:"size:7;uniqueIndex:idx_settlement_month_team;not null" json:"month"`
	RepairTeam string `gorm:"size:64;uniqueIndex:idx_settlement_month_team;not null" json:"repair_team"`

	TotalCount int64          `json:"total_count"`
	TotalCost  float64        `json:"total_cost"`
	ByResult   []ResultBucket `gorm:"serializer:json" json:"by_result"`

	SettledBy string    `gorm:"size:64;not null" json:"settled_by"`
	SettledAt time.Time `json:"settled_at"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名。
func (TeamMonthSettlement) TableName() string { return "settlement_month" }
