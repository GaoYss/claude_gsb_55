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
// Result 与 FinishedAt 是完工时的历史事实, 落库后不再改写;
// 结果更正通过 RepairCorrection 修订记录进行, 统计口径使用 CurrentResult 等生效字段。
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

	// 生效结果(统计口径): 完工时与 Result 一致, 每次结果更正后刷新为最新取值。
	CurrentResult    string     `gorm:"size:32;index" json:"current_result"`
	CurrentLiability string     `gorm:"size:32;index" json:"current_liability"`
	CurrentNature    string     `gorm:"size:32;index" json:"current_nature"`
	CorrectionCount  int        `gorm:"not null;default:0" json:"correction_count"`
	LastCorrectedAt  *time.Time `json:"last_corrected_at"`

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

// EffectiveResult 返回当前生效的维修结果, 未完工或未初始化时回退到原始结果。
func (r *Repair) EffectiveResult() string {
	if r.CurrentResult != "" {
		return r.CurrentResult
	}
	return r.Result
}

// ApplyEffectiveResult 把生效结果及其对应的责任方、维修性质写回记录。
func (r *Repair) ApplyEffectiveResult(result string) {
	r.CurrentResult = result
	r.CurrentLiability, r.CurrentNature = ResultAttribution(result)
}

// RepairCorrection 维修结果更正记录, 是可追溯的修订路径上的一环。
// 每次更正追加一条, 记录更正前后的结果/责任方/维修性质, 永不修改与删除。
type RepairCorrection struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	BatchNo       string    `gorm:"size:64;index;not null" json:"batch_no"` // 更正批次号 ZG+YYYYMMDD+序号
	RepairID      uint      `gorm:"index;not null" json:"repair_id"`
	RepairNo      string    `gorm:"size:64;index" json:"repair_no"`
	FaultID       uint      `gorm:"index" json:"fault_id"`
	RepairTeam    string    `gorm:"size:64;index" json:"repair_team"` // 更正时维修记录所属班组(快照)
	OldResult     string    `gorm:"size:32" json:"old_result"`
	NewResult     string    `gorm:"size:32" json:"new_result"`
	OldLiability  string    `gorm:"size:32" json:"old_liability"`
	NewLiability  string    `gorm:"size:32" json:"new_liability"`
	OldNature     string    `gorm:"size:32" json:"old_nature"`
	NewNature     string    `gorm:"size:32" json:"new_nature"`
	Cost          float64   `json:"cost"` // 更正时维修费用(快照), 用于跨月调整的金额归集
	Reason        string    `gorm:"size:255;not null" json:"reason"`
	Operator      string    `gorm:"size:64;index;not null" json:"operator"`
	SourceMonth   string    `gorm:"size:7;index" json:"source_month"` // 完工时间所在月份 YYYY-MM
	EffectMonth   string    `gorm:"size:7;index" json:"effect_month"` // 更正生效月份(更正发生月) YYYY-MM
	SourceSettled bool      `gorm:"index" json:"source_settled"`      // 源月份在更正时是否已结算
	CreatedAt     time.Time `json:"created_at"`
}

// TableName 指定表名。
func (RepairCorrection) TableName() string { return "repair_correction" }

// RepairMonthSettlement 维修月度结算, 结算后该月的班组归集金额封存不再变化。
type RepairMonthSettlement struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Month     string    `gorm:"size:7;uniqueIndex;not null" json:"month"` // 结算月份 YYYY-MM
	SettledBy string    `gorm:"size:64;not null" json:"settled_by"`
	Note      string    `gorm:"size:255" json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名。
func (RepairMonthSettlement) TableName() string { return "repair_month_settlement" }

// RepairSettlementRow 结算时封存的班组归集快照行, 按 班组 × 责任方 × 维修性质 分组。
type RepairSettlementRow struct {
	ID          uint    `gorm:"primaryKey" json:"id"`
	Month       string  `gorm:"size:7;index;not null" json:"month"`
	RepairTeam  string  `gorm:"size:64;index" json:"repair_team"`
	Liability   string  `gorm:"size:32" json:"liability"`
	Nature      string  `gorm:"size:32" json:"nature"`
	RepairCount int64   `json:"repair_count"`
	TotalCost   float64 `json:"total_cost"`
}

// TableName 指定表名。
func (RepairSettlementRow) TableName() string { return "repair_settlement_row" }
