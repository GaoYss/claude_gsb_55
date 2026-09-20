package repair

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"streetlight/internal/apperr"
	"streetlight/pkg/pagination"
)

// correctionSortSpec 定义更正记录列表允许的排序字段白名单。
var correctionSortSpec = pagination.SortSpec{
	Allowed: map[string]string{
		"batch_no":     "batch_no",
		"repair_no":    "repair_no",
		"effect_month": "effect_month",
		"source_month": "source_month",
		"created_at":   "created_at",
	},
	Default: "created_at",
}

// CorrectionFilter 是仓储层使用的更正记录查询条件。
type CorrectionFilter struct {
	RepairID uint
	BatchNo  string
	Month    string
	Operator string
}

// ---------------------------------------------------------------------------
// 仓储: 更正记录 / 月度结算 / 班组归集
// ---------------------------------------------------------------------------

// Transaction 在单个数据库事务内执行 fn。
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// GetForUpdateTx 在事务内按主键锁定并读取维修记录。
func (r *Repository) GetForUpdateTx(tx *gorm.DB, id uint) (*Repair, error) {
	var entity Repair
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&entity, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.NotFound("维修记录不存在: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询维修记录失败: %w", err)
	}
	return &entity, nil
}

// CreateCorrectionTx 在事务内追加一条更正记录。
func (r *Repository) CreateCorrectionTx(tx *gorm.DB, entity *RepairCorrection) error {
	if err := tx.Create(entity).Error; err != nil {
		return fmt.Errorf("写入更正记录失败: %w", err)
	}
	return nil
}

// ApplyCorrectionTx 在事务内把更正后的生效结果写回维修记录, 并累加更正次数。
func (r *Repository) ApplyCorrectionTx(tx *gorm.DB, entity *Repair, correctedAt time.Time) error {
	columns := map[string]any{
		"current_result":    entity.CurrentResult,
		"current_liability": entity.CurrentLiability,
		"current_nature":    entity.CurrentNature,
		"correction_count":  entity.CorrectionCount,
		"last_corrected_at": correctedAt,
	}
	if err := tx.Model(&Repair{}).Where("id = ?", entity.ID).Updates(columns).Error; err != nil {
		return fmt.Errorf("回写生效维修结果失败: %w", err)
	}
	return nil
}

// NextBatchSequence 返回更正批次号前缀下的下一个流水号。
func (r *Repository) NextBatchSequence(ctx context.Context, prefix string) (int, error) {
	var latest string
	err := r.session(ctx).Model(&RepairCorrection{}).
		Where("batch_no LIKE ?", prefix+"%").
		Order("batch_no DESC").
		Limit(1).
		Pluck("batch_no", &latest).Error
	if err != nil {
		return 0, fmt.Errorf("生成更正批次号失败: %w", err)
	}
	if latest == "" {
		return 1, nil
	}
	var value int
	if _, convErr := fmt.Sscanf(strings.TrimPrefix(latest, prefix), "%d", &value); convErr != nil {
		return 1, nil
	}
	return value + 1, nil
}

// ListCorrections 分页查询更正记录。
func (r *Repository) ListCorrections(ctx context.Context, filter CorrectionFilter, page pagination.Query) ([]RepairCorrection, int64, error) {
	base := func() *gorm.DB {
		statement := r.session(ctx).Model(&RepairCorrection{})
		if filter.RepairID > 0 {
			statement = statement.Where("repair_id = ?", filter.RepairID)
		}
		if value := strings.TrimSpace(filter.BatchNo); value != "" {
			statement = statement.Where("batch_no = ?", value)
		}
		if value := strings.TrimSpace(filter.Month); value != "" {
			statement = statement.Where("effect_month = ?", value)
		}
		if value := strings.TrimSpace(filter.Operator); value != "" {
			statement = statement.Where("operator = ?", value)
		}
		return statement
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计更正记录失败: %w", err)
	}

	entities := make([]RepairCorrection, 0)
	if err := base().Order(page.OrderClause()).Offset(page.Offset()).Limit(page.Limit()).Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("查询更正记录失败: %w", err)
	}
	return entities, total, nil
}

// ListCorrectionsByRepair 查询某条维修记录的全部更正记录, 按时间正序形成修订路径。
func (r *Repository) ListCorrectionsByRepair(ctx context.Context, repairID uint) ([]RepairCorrection, error) {
	entities := make([]RepairCorrection, 0)
	err := r.session(ctx).
		Where("repair_id = ?", repairID).
		Order("created_at ASC, id ASC").
		Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询维修记录更正历史失败: %w", err)
	}
	return entities, nil
}

// ListSettledSourceCorrections 查询生效月份为 effectMonth 且源月份已结算的更正,
// 这些更正的金额影响无法回到源月, 作为调整项计入更正发生月的归集。
func (r *Repository) ListSettledSourceCorrections(ctx context.Context, effectMonth string) ([]RepairCorrection, error) {
	entities := make([]RepairCorrection, 0)
	err := r.session(ctx).
		Where("effect_month = ? AND source_settled = ?", effectMonth, true).
		Order("created_at ASC, id ASC").
		Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询跨月更正调整失败: %w", err)
	}
	return entities, nil
}

// CountCorrectionsByRepair 统计某条维修记录的更正次数。
func (r *Repository) CountCorrectionsByRepair(ctx context.Context, repairID uint) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&RepairCorrection{}).Where("repair_id = ?", repairID).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计更正记录失败: %w", err)
	}
	return count, nil
}

// AggregateFinishedByTeam 按 班组 × 责任方 × 维修性质 实时归集完工月份落在 [from, to) 的维修记录,
// 口径为当前生效结果, 结果更正后再次查询即同步刷新。
func (r *Repository) AggregateFinishedByTeam(ctx context.Context, from, to time.Time) ([]AggregationRow, error) {
	rows := make([]AggregationRow, 0)
	err := r.session(ctx).Model(&Repair{}).
		Select("repair_team, current_liability AS liability, current_nature AS nature, COUNT(*) AS repair_count, COALESCE(SUM(cost), 0) AS total_cost").
		Where("status = ? AND finished_at >= ? AND finished_at < ?", StatusFinished, from, to).
		Group("repair_team, current_liability, current_nature").
		Order("repair_team, liability, nature").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("按班组归集维修记录失败: %w", err)
	}
	return rows, nil
}

// GetSettlement 查询某月的结算记录, 未结算时返回 nil。
func (r *Repository) GetSettlement(ctx context.Context, month string) (*RepairMonthSettlement, error) {
	var entity RepairMonthSettlement
	err := r.session(ctx).Where("month = ?", month).First(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询月度结算失败: %w", err)
	}
	return &entity, nil
}

// ListSettlements 查询全部已结算月份, 按月份倒序。
func (r *Repository) ListSettlements(ctx context.Context) ([]RepairMonthSettlement, error) {
	entities := make([]RepairMonthSettlement, 0)
	err := r.session(ctx).Order("month DESC").Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询月度结算列表失败: %w", err)
	}
	return entities, nil
}

// CreateSettlementTx 在事务内写入结算记录与归集快照行。
func (r *Repository) CreateSettlementTx(tx *gorm.DB, settlement *RepairMonthSettlement, rows []RepairSettlementRow) error {
	if err := tx.Create(settlement).Error; err != nil {
		if isUniqueViolation(err) {
			return apperr.Conflict("月份 %s 已结算, 请勿重复操作", settlement.Month)
		}
		return fmt.Errorf("写入月度结算失败: %w", err)
	}
	for index := range rows {
		rows[index].Month = settlement.Month
	}
	if len(rows) > 0 {
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("写入结算归集快照失败: %w", err)
		}
	}
	return nil
}

// ListSettlementRows 查询某月结算时封存的归集快照。
func (r *Repository) ListSettlementRows(ctx context.Context, month string) ([]RepairSettlementRow, error) {
	rows := make([]RepairSettlementRow, 0)
	err := r.session(ctx).
		Where("month = ?", month).
		Order("repair_team, liability, nature").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询结算归集快照失败: %w", err)
	}
	return rows, nil
}

// CountFinishedByColumn 按列分组统计已完工记录(口径为生效结果相关列), 空值不计入。
func (r *Repository) CountFinishedByColumn(ctx context.Context, column string) (map[string]int64, error) {
	type row struct {
		Label string
		Total int64
	}
	rows := make([]row, 0)
	err := r.session(ctx).Model(&Repair{}).
		Select(column+" AS label, COUNT(*) AS total").
		Where("status = ? AND "+column+" <> ''", StatusFinished).
		Group(column).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("分组统计 %s 失败: %w", column, err)
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Label] = item.Total
	}
	return result, nil
}

// BackfillEffectiveResults 把历史已完工记录的生效结果初始化为原始结果, 幂等。
func (r *Repository) BackfillEffectiveResults(ctx context.Context) error {
	for _, result := range Results() {
		liability, nature := ResultAttribution(result)
		err := r.session(ctx).Model(&Repair{}).
			Where("status = ? AND result = ? AND (current_result IS NULL OR current_result = '')", StatusFinished, result).
			Updates(map[string]any{
				"current_result":    result,
				"current_liability": liability,
				"current_nature":    nature,
			}).Error
		if err != nil {
			return fmt.Errorf("回填生效维修结果失败: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 服务: 结果更正(可追溯修订路径)
// ---------------------------------------------------------------------------

// Correct 单笔结果更正, 等价于长度为 1 的原子批量更正。
func (s *Service) Correct(ctx context.Context, id uint, item CorrectItem, operator string) (*CorrectBatchResult, error) {
	item.RepairID = id
	return s.CorrectBatch(ctx, CorrectBatchRequest{
		Operator: operator,
		Mode:     CorrectModeAtomic,
		Items:    []CorrectItem{item},
	})
}

// CorrectBatch 批量结果更正。
//
// 处理模式(不满足条件时的行为必须在响应中说清):
//   - atomic(默认): 任一条不满足条件则整批不动, 返回 409 并逐条说明原因;
//   - partial: 逐条处理, 每条独立事务, 成功与失败在结果的 succeeded / failed 中分别返回。
//
// 每笔更正都会追加一条 RepairCorrection 修订记录并刷新维修记录的生效结果,
// 原始结果与完工时间保持不变; 源月份已结算的跨月更正作为调整项计入更正发生月。
func (s *Service) CorrectBatch(ctx context.Context, req CorrectBatchRequest) (*CorrectBatchResult, error) {
	operator := strings.TrimSpace(req.Operator)
	if operator == "" {
		return nil, apperr.BadRequest("更正操作人不能为空")
	}
	if len(req.Items) == 0 {
		return nil, apperr.BadRequest("批量更正至少需要一条明细")
	}

	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = CorrectModeAtomic
	}
	if mode != CorrectModeAtomic && mode != CorrectModePartial {
		return nil, apperr.BadRequest("非法的批量处理模式: %s", mode)
	}

	now := time.Now()
	effectMonth := now.Format("2006-01")

	// 更正归集写入更正发生月, 该月已结算时任何更正都无法落账。
	if settled, err := s.monthSettled(ctx, effectMonth); err != nil {
		return nil, err
	} else if settled {
		return nil, apperr.Conflict("更正生效月份 %s 已结算, 无法登记结果更正", effectMonth)
	}

	if mode == CorrectModeAtomic {
		return s.correctBatchAtomic(ctx, req.Items, operator, effectMonth, now)
	}
	return s.correctBatchPartial(ctx, req.Items, operator, effectMonth, now)
}

// correctBatchAtomic 原子批量: 先整批校验, 任一失败则整批不动; 全部通过后在单个事务内写入。
func (s *Service) correctBatchAtomic(ctx context.Context, items []CorrectItem, operator, effectMonth string, now time.Time) (*CorrectBatchResult, error) {
	repairs := make([]*Repair, len(items))
	failures := make([]CorrectFailure, 0)
	for index, item := range items {
		entity, err := s.validateCorrectItem(ctx, item, index)
		if err != nil {
			failures = append(failures, *err)
			continue
		}
		repairs[index] = entity
	}
	if len(failures) > 0 {
		return nil, apperr.Conflict("批量更正未执行(整批不动), 共 %d 条不满足条件: %s", len(failures), joinFailures(failures))
	}

	batchNo, err := s.nextBatchNo(ctx, now)
	if err != nil {
		return nil, err
	}

	result := &CorrectBatchResult{Mode: CorrectModeAtomic, BatchNo: batchNo, Succeeded: make([]CorrectionView, 0, len(items)), Failed: make([]CorrectFailure, 0)}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		for index, item := range items {
			correction, err := s.applyCorrectionTx(tx, repairs[index].ID, item, operator, batchNo, effectMonth, now)
			if err != nil {
				return err
			}
			result.Succeeded = append(result.Succeeded, toCorrectionView(correction))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.Applied = len(result.Succeeded)
	return result, nil
}

// correctBatchPartial 逐条处理: 每条明细独立事务, 互不影响, 成功与失败逐条返回。
func (s *Service) correctBatchPartial(ctx context.Context, items []CorrectItem, operator, effectMonth string, now time.Time) (*CorrectBatchResult, error) {
	result := &CorrectBatchResult{Mode: CorrectModePartial, Succeeded: make([]CorrectionView, 0), Failed: make([]CorrectFailure, 0)}

	batchNo, err := s.nextBatchNo(ctx, now)
	if err != nil {
		return nil, err
	}

	for index, item := range items {
		entity, failure := s.validateCorrectItem(ctx, item, index)
		if failure != nil {
			result.Failed = append(result.Failed, *failure)
			continue
		}

		var correction *RepairCorrection
		err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
			var txErr error
			correction, txErr = s.applyCorrectionTx(tx, entity.ID, item, operator, batchNo, effectMonth, now)
			return txErr
		})
		if err != nil {
			result.Failed = append(result.Failed, CorrectFailure{
				Index:    index,
				RepairID: item.RepairID,
				RepairNo: entity.RepairNo,
				Message:  errorMessage(err),
			})
			continue
		}
		result.Succeeded = append(result.Succeeded, toCorrectionView(correction))
	}

	if len(result.Succeeded) > 0 {
		result.BatchNo = batchNo
	}
	result.Applied = len(result.Succeeded)
	return result, nil
}

// validateCorrectItem 读取维修记录并校验更正条件, 失败时转换为逐条失败信息。
func (s *Service) validateCorrectItem(ctx context.Context, item CorrectItem, index int) (*Repair, *CorrectFailure) {
	wrap := func(err error, repairNo string) *CorrectFailure {
		return &CorrectFailure{Index: index, RepairID: item.RepairID, RepairNo: repairNo, Message: errorMessage(err)}
	}

	if item.RepairID == 0 {
		return nil, wrap(apperr.BadRequest("维修记录 ID 不能为空"), "")
	}
	entity, err := s.repo.GetByID(ctx, item.RepairID)
	if err != nil {
		return nil, wrap(err, "")
	}
	if err := checkCorrectable(entity, item.NewResult, item.Reason); err != nil {
		return nil, wrap(err, entity.RepairNo)
	}
	return entity, nil
}

// checkCorrectable 校验一条维修记录是否满足更正条件。
func checkCorrectable(entity *Repair, newResult, reason string) error {
	if entity.Status != StatusFinished {
		return apperr.Conflict("维修记录 %s 尚未完工, 不能更正维修结果", entity.RepairNo)
	}
	newResult = strings.TrimSpace(newResult)
	if !IsValidResult(newResult) {
		return apperr.BadRequest("非法的维修结果: %s", newResult)
	}
	if newResult == entity.EffectiveResult() {
		return apperr.Conflict("维修记录 %s 当前生效结果已是 %s, 无需更正", entity.RepairNo, ResultLabel(newResult))
	}
	if strings.TrimSpace(reason) == "" {
		return apperr.BadRequest("维修记录 %s 的更正原因不能为空", entity.RepairNo)
	}
	return nil
}

// applyCorrectionTx 在事务内完成一笔更正: 复核状态、追加修订记录、刷新生效结果。
func (s *Service) applyCorrectionTx(tx *gorm.DB, repairID uint, item CorrectItem, operator, batchNo, effectMonth string, now time.Time) (*RepairCorrection, error) {
	entity, err := s.repo.GetForUpdateTx(tx, repairID)
	if err != nil {
		return nil, err
	}
	if err := checkCorrectable(entity, item.NewResult, item.Reason); err != nil {
		return nil, err
	}

	newResult := strings.TrimSpace(item.NewResult)
	oldResult := entity.EffectiveResult()
	oldLiability, oldNature := ResultAttribution(oldResult)
	newLiability, newNature := ResultAttribution(newResult)

	sourceMonth := entity.FinishedAt.Format("2006-01")
	sourceSettled, err := s.monthSettledTx(tx, sourceMonth)
	if err != nil {
		return nil, err
	}

	correction := &RepairCorrection{
		BatchNo:       batchNo,
		RepairID:      entity.ID,
		RepairNo:      entity.RepairNo,
		FaultID:       entity.FaultID,
		RepairTeam:    entity.RepairTeam,
		OldResult:     oldResult,
		NewResult:     newResult,
		OldLiability:  oldLiability,
		NewLiability:  newLiability,
		OldNature:     oldNature,
		NewNature:     newNature,
		Cost:          entity.Cost,
		Reason:        strings.TrimSpace(item.Reason),
		Operator:      operator,
		SourceMonth:   sourceMonth,
		EffectMonth:   effectMonth,
		SourceSettled: sourceSettled,
	}
	if err := s.repo.CreateCorrectionTx(tx, correction); err != nil {
		return nil, err
	}

	entity.ApplyEffectiveResult(newResult)
	entity.CorrectionCount++
	if err := s.repo.ApplyCorrectionTx(tx, entity, now); err != nil {
		return nil, err
	}
	return correction, nil
}

// ListCorrections 分页查询更正记录, 每条附带更正前后的逐字段对照。
func (s *Service) ListCorrections(ctx context.Context, query CorrectionListQuery) ([]CorrectionView, int64, pagination.Query, error) {
	page := pagination.Parse(query.Params, correctionSortSpec)
	filter := CorrectionFilter{
		RepairID: query.RepairID,
		BatchNo:  strings.TrimSpace(query.BatchNo),
		Operator: strings.TrimSpace(query.Operator),
	}
	if value := strings.TrimSpace(query.Month); value != "" {
		if _, err := parseMonth(value); err != nil {
			return nil, 0, page, err
		}
		filter.Month = value
	}

	entities, total, err := s.repo.ListCorrections(ctx, filter, page)
	if err != nil {
		return nil, 0, page, err
	}
	views := make([]CorrectionView, 0, len(entities))
	for index := range entities {
		views = append(views, toCorrectionView(&entities[index]))
	}
	return views, total, page, nil
}

// ListRepairCorrections 查询某条维修记录的完整修订路径(按时间正序)。
func (s *Service) ListRepairCorrections(ctx context.Context, repairID uint) ([]CorrectionView, error) {
	if _, err := s.repo.GetByID(ctx, repairID); err != nil {
		return nil, err
	}
	entities, err := s.repo.ListCorrectionsByRepair(ctx, repairID)
	if err != nil {
		return nil, err
	}
	views := make([]CorrectionView, 0, len(entities))
	for index := range entities {
		views = append(views, toCorrectionView(&entities[index]))
	}
	return views, nil
}

// ---------------------------------------------------------------------------
// 服务: 班组月度归集与结算
// ---------------------------------------------------------------------------

// MonthlyAggregation 查询某月的班组归集。
// 未结算月按当前生效结果实时归集, 结果更正后同步刷新;
// 已结算月返回结算时封存的快照, 之后的跨月更正以调整项形式计入其更正发生月。
func (s *Service) MonthlyAggregation(ctx context.Context, month string) (*AggregationResponse, error) {
	from, err := parseMonth(month)
	if err != nil {
		return nil, err
	}
	to := from.AddDate(0, 1, 0)

	response := &AggregationResponse{
		Month:       from.Format("2006-01"),
		Rows:        make([]AggregationRow, 0),
		Adjustments: make([]AdjustmentView, 0),
	}

	settlement, err := s.repo.GetSettlement(ctx, response.Month)
	if err != nil {
		return nil, err
	}
	if settlement != nil {
		response.Settled = true
		response.SettledBy = settlement.SettledBy
		settledAt := settlement.CreatedAt
		response.SettledAt = &settledAt

		snapshot, err := s.repo.ListSettlementRows(ctx, response.Month)
		if err != nil {
			return nil, err
		}
		for _, row := range snapshot {
			response.Rows = append(response.Rows, AggregationRow{
				RepairTeam:  row.RepairTeam,
				Liability:   row.Liability,
				Nature:      row.Nature,
				RepairCount: row.RepairCount,
				TotalCost:   row.TotalCost,
			})
		}
	} else {
		rows, err := s.repo.AggregateFinishedByTeam(ctx, from, to)
		if err != nil {
			return nil, err
		}
		response.Rows = rows
	}

	// 跨月更正: 源月份已结算的更正无法回写源月归集, 作为调整项计入更正发生月。
	adjustments, err := s.repo.ListSettledSourceCorrections(ctx, response.Month)
	if err != nil {
		return nil, err
	}
	for index := range adjustments {
		response.Adjustments = append(response.Adjustments, toAdjustmentView(&adjustments[index]))
	}
	return response, nil
}

// SettleMonth 结算某月: 按当前生效结果封存班组归集快照, 之后该月归集金额不再变化。
func (s *Service) SettleMonth(ctx context.Context, req SettleMonthRequest) (*AggregationResponse, error) {
	month := strings.TrimSpace(req.Month)
	from, err := parseMonth(month)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	if from.After(currentMonth) {
		return nil, apperr.BadRequest("不能结算未来月份: %s", month)
	}
	if settledBy := strings.TrimSpace(req.SettledBy); settledBy == "" {
		return nil, apperr.BadRequest("结算人不能为空")
	}

	existing, err := s.repo.GetSettlement(ctx, month)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperr.Conflict("月份 %s 已结算, 归集金额已封存", month)
	}

	rows, err := s.repo.AggregateFinishedByTeam(ctx, from, from.AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}

	settlement := &RepairMonthSettlement{
		Month:     month,
		SettledBy: strings.TrimSpace(req.SettledBy),
		Note:      strings.TrimSpace(req.Note),
	}
	snapshot := make([]RepairSettlementRow, 0, len(rows))
	for _, row := range rows {
		snapshot = append(snapshot, RepairSettlementRow{
			RepairTeam:  row.RepairTeam,
			Liability:   row.Liability,
			Nature:      row.Nature,
			RepairCount: row.RepairCount,
			TotalCost:   row.TotalCost,
		})
	}

	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateSettlementTx(tx, settlement, snapshot)
	})
	if err != nil {
		return nil, err
	}
	return s.MonthlyAggregation(ctx, month)
}

// ListSettlements 查询全部已结算月份。
func (s *Service) ListSettlements(ctx context.Context) ([]RepairMonthSettlement, error) {
	return s.repo.ListSettlements(ctx)
}

// ---------------------------------------------------------------------------
// 内部工具
// ---------------------------------------------------------------------------

// monthSettled 判断某月是否已结算。
func (s *Service) monthSettled(ctx context.Context, month string) (bool, error) {
	settlement, err := s.repo.GetSettlement(ctx, month)
	if err != nil {
		return false, err
	}
	return settlement != nil, nil
}

// monthSettledTx 在事务内判断某月是否已结算。
func (s *Service) monthSettledTx(tx *gorm.DB, month string) (bool, error) {
	var count int64
	if err := tx.Model(&RepairMonthSettlement{}).Where("month = ?", month).Count(&count).Error; err != nil {
		return false, fmt.Errorf("查询月度结算失败: %w", err)
	}
	return count > 0, nil
}

// nextBatchNo 生成更正批次号 ZG+YYYYMMDD+4 位序号。
func (s *Service) nextBatchNo(ctx context.Context, now time.Time) (string, error) {
	prefix := "ZG" + now.Format("20060102")
	sequence, err := s.repo.NextBatchSequence(ctx, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%04d", prefix, sequence), nil
}

// toCorrectionView 组装更正记录视图, 附带更正前后的逐字段对照。
func toCorrectionView(entity *RepairCorrection) CorrectionView {
	return CorrectionView{
		RepairCorrection: *entity,
		Changes: []FieldChange{
			{
				Field: "result", FieldLabel: "维修结果",
				Before: entity.OldResult, After: entity.NewResult,
				BeforeLabel: ResultLabel(entity.OldResult), AfterLabel: ResultLabel(entity.NewResult),
			},
			{
				Field: "liability", FieldLabel: "责任方",
				Before: entity.OldLiability, After: entity.NewLiability,
				BeforeLabel: LiabilityLabel(entity.OldLiability), AfterLabel: LiabilityLabel(entity.NewLiability),
			},
			{
				Field: "nature", FieldLabel: "维修性质",
				Before: entity.OldNature, After: entity.NewNature,
				BeforeLabel: NatureLabel(entity.OldNature), AfterLabel: NatureLabel(entity.NewNature),
			},
		},
	}
}

// toAdjustmentView 把源月已结算的更正转换为当月的调整项视图。
func toAdjustmentView(entity *RepairCorrection) AdjustmentView {
	return AdjustmentView{
		CorrectionID: entity.ID,
		BatchNo:      entity.BatchNo,
		RepairID:     entity.RepairID,
		RepairNo:     entity.RepairNo,
		RepairTeam:   entity.RepairTeam,
		SourceMonth:  entity.SourceMonth,
		OldResult:    entity.OldResult,
		NewResult:    entity.NewResult,
		OldLiability: entity.OldLiability,
		NewLiability: entity.NewLiability,
		OldNature:    entity.OldNature,
		NewNature:    entity.NewNature,
		Cost:         entity.Cost,
		Reason:       entity.Reason,
		Operator:     entity.Operator,
		CreatedAt:    entity.CreatedAt,
	}
}

// joinFailures 把逐条失败信息拼接为一段说明, 用于整批拒绝时的错误提示。
func joinFailures(failures []CorrectFailure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		label := failure.RepairNo
		if label == "" {
			label = fmt.Sprintf("id=%d", failure.RepairID)
		}
		parts = append(parts, fmt.Sprintf("第 %d 条(%s): %s", failure.Index+1, label, failure.Message))
	}
	return strings.Join(parts, "; ")
}

// errorMessage 提取面向使用者的错误信息。
func errorMessage(err error) string {
	if businessErr, ok := apperr.As(err); ok {
		return businessErr.Message
	}
	return err.Error()
}

// parseMonth 解析 YYYY-MM 月份, 返回当月第一天零点。
func parseMonth(value string) (time.Time, error) {
	month, err := time.ParseInLocation("2006-01", strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, apperr.BadRequest("月份格式应为 YYYY-MM, 当前值: %s", value)
	}
	return month, nil
}
