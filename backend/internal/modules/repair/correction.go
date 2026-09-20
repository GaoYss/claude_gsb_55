package repair

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"streetlight/internal/apperr"
)

// 更正相关单号前缀: 修订单 XD, 批次 PC, 与故障单 GD / 维修单 WX 风格一致。
const (
	revisionNoPrefix = "XD"
	batchNoPrefix    = "PC"
)

// maxCorrectionAttempts 是单号冲突时的最大重试次数。
const maxCorrectionAttempts = 5

// correctionPlan 是一条更正通过校验后的落库计划。
type correctionPlan struct {
	repair         *Repair
	newResult      string
	newCost        float64
	changes        []FieldChange
	originMonth    string
	effectiveMonth string
	crossSettled   bool
	syncFault      bool // 是否需要在落库后联动故障状态
	newFixed       bool // 更正后的结果是否为已修复
}

// Correct 执行一批维修结果更正。
//
// 批量语义: 整批原子 —— 任一条目不满足条件时整批不执行, 返回 BatchCorrectError
// 并逐条列出失败明细; 全部通过时在单个事务内写入生效值与修订记录。
// 历史留痕: 原维修记录的 result / finished_at / cost 不被改写, 统计口径切换到生效值。
func (s *Service) Correct(ctx context.Context, req CorrectRequest) (*CorrectResult, error) {
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, apperr.BadRequest("更正原因不能为空")
	}
	operator := strings.TrimSpace(req.Operator)
	if operator == "" {
		return nil, apperr.BadRequest("操作人不能为空")
	}
	if len(req.Items) == 0 {
		return nil, apperr.BadRequest("更正条目不能为空")
	}
	if len(req.Items) > 100 {
		return nil, apperr.BadRequest("单批更正最多 100 条")
	}

	now := time.Now()
	currentMonth := now.Format("2006-01")

	// 先逐条校验并组装落库计划, 收集全部失败明细, 不中途返回。
	plans := make([]correctionPlan, 0, len(req.Items))
	failures := make([]CorrectItemError, 0)
	seen := make(map[uint]bool, len(req.Items))
	for _, item := range req.Items {
		if seen[item.RepairID] {
			failures = append(failures, CorrectItemError{RepairID: item.RepairID, Message: "同一批次中重复更正同一条维修记录"})
			continue
		}
		seen[item.RepairID] = true

		plan, itemErr := s.prepareCorrection(ctx, item, now, currentMonth)
		if itemErr != nil {
			failures = append(failures, *itemErr)
			continue
		}
		plans = append(plans, *plan)
	}
	if len(failures) > 0 {
		return nil, &BatchCorrectError{Items: failures}
	}

	// 生成批次号与修订单号, 唯一索引冲突时整批重取序号重试。
	var revisions []RepairRevision
	var batchNo string
	var lastErr error
	for attempt := 0; attempt < maxCorrectionAttempts; attempt++ {
		batchNo, revisions, lastErr = s.buildRevisions(ctx, plans, reason, operator, now, attempt)
		if lastErr != nil {
			break
		}
		writes := make([]CorrectionWrite, 0, len(plans))
		for index := range plans {
			writes = append(writes, CorrectionWrite{
				RepairID: plans[index].repair.ID,
				Updates: map[string]any{
					"current_result": plans[index].newResult,
					"current_cost":   plans[index].newCost,
					"revision_count": plans[index].repair.RevisionCount + 1,
				},
				Revision: &revisions[index],
			})
		}
		if lastErr = s.repo.ApplyCorrections(ctx, writes); lastErr == nil {
			break
		}
		if !isUniqueViolation(lastErr) {
			return nil, lastErr
		}
	}
	if lastErr != nil {
		if isUniqueViolation(lastErr) {
			return nil, apperr.Conflict("修订单号生成冲突, 请稍后重试")
		}
		return nil, lastErr
	}

	// 故障状态联动放在更正事务之后, 与完工联动的写法保持一致(最终一致)。
	for index := range plans {
		plan := &plans[index]
		if !plan.syncFault {
			continue
		}
		if err := s.faults.OnRepairResultCorrected(ctx, plan.repair.FaultID, plan.newFixed); err != nil {
			return nil, fmt.Errorf("更正已生效, 但联动故障状态失败: %w", err)
		}
	}

	return &CorrectResult{BatchNo: batchNo, Revisions: revisions}, nil
}

// prepareCorrection 校验单条更正并组装落库计划, 失败时返回逐条错误明细。
func (s *Service) prepareCorrection(ctx context.Context, item CorrectItem, now time.Time, currentMonth string) (*correctionPlan, *CorrectItemError) {
	fail := func(repairNo, format string, args ...any) (*correctionPlan, *CorrectItemError) {
		return nil, &CorrectItemError{
			RepairID: item.RepairID,
			RepairNo: repairNo,
			Message:  fmt.Sprintf(format, args...),
		}
	}

	entity, err := s.repo.GetByID(ctx, item.RepairID)
	if err != nil {
		if appErr, ok := apperr.As(err); ok {
			return fail("", "%s", appErr.Message)
		}
		return fail("", "查询维修记录失败")
	}
	if entity.Status != StatusFinished {
		return fail(entity.RepairNo, "维修记录尚未完工, 没有可更正的维修结果")
	}
	if item.Result == nil && item.Cost == nil {
		return fail(entity.RepairNo, "未提供要更正的字段, 结果与金额至少更正一项")
	}

	newResult := entity.EffectiveResult()
	if item.Result != nil {
		candidate := strings.TrimSpace(*item.Result)
		if !IsValidResult(candidate) {
			return fail(entity.RepairNo, "非法的维修结果: %s", candidate)
		}
		newResult = candidate
	}
	newCost := entity.EffectiveCost()
	if item.Cost != nil {
		if *item.Cost < 0 {
			return fail(entity.RepairNo, "更正后的金额不能为负数")
		}
		newCost = *item.Cost
	}

	changes := make([]FieldChange, 0, 2)
	if newResult != entity.EffectiveResult() {
		changes = append(changes, FieldChange{
			Field:    "result",
			Label:    "维修结果",
			OldValue: ResultLabel(entity.EffectiveResult()),
			NewValue: ResultLabel(newResult),
		})
	}
	if newCost != entity.EffectiveCost() {
		changes = append(changes, FieldChange{
			Field:    "cost",
			Label:    "维修金额",
			OldValue: strconv.FormatFloat(entity.EffectiveCost(), 'f', 2, 64),
			NewValue: strconv.FormatFloat(newCost, 'f', 2, 64),
		})
	}
	if len(changes) == 0 {
		return fail(entity.RepairNo, "更正内容与原生效值一致, 未发生变化")
	}

	// 归集口径: 原完工月未结算时更正直接反映在原月;
	// 原完工月已结算时历史月金额冻结, 本次更正以调整项计入当前开放月。
	originMonth := entity.FinishedAt.Format("2006-01")
	plan := &correctionPlan{
		repair:         entity,
		newResult:      newResult,
		newCost:        newCost,
		changes:        changes,
		originMonth:    originMonth,
		effectiveMonth: originMonth,
		newFixed:       newResult == ResultFixed,
	}
	settled, err := s.repo.IsMonthSettled(ctx, originMonth)
	if err != nil {
		return fail(entity.RepairNo, "查询月度结算状态失败")
	}
	if settled {
		currentSettled, err := s.repo.IsMonthSettled(ctx, currentMonth)
		if err != nil {
			return fail(entity.RepairNo, "查询月度结算状态失败")
		}
		if currentSettled {
			return fail(entity.RepairNo, "原完工月 %s 与当前月 %s 均已完成结算, 没有可登记调整的开放月份", originMonth, currentMonth)
		}
		plan.effectiveMonth = currentMonth
		plan.crossSettled = true
	}

	// 仅当更正的是故障当前最新一条维修记录的结果时, 才需要联动故障状态;
	// 已关闭的故障由人工闭环, 更正只影响统计口径, 不重开故障。
	if newResult != entity.EffectiveResult() {
		target, err := s.faults.GetByID(ctx, entity.FaultID)
		if err != nil {
			return fail(entity.RepairNo, "查询关联故障失败")
		}
		if target.LatestRepairID != nil && *target.LatestRepairID == entity.ID {
			plan.syncFault = true
		}
	}
	return plan, nil
}

// buildRevisions 为一批更正计划生成批次号与修订单号并组装修订记录。
// bump 是单号冲突重试时的整体偏移量。
func (s *Service) buildRevisions(ctx context.Context, plans []correctionPlan, reason, operator string, now time.Time, bump int) (string, []RepairRevision, error) {
	dateStamp := now.Format("20060102")

	batchSeq, err := s.repo.NextBatchSequence(ctx, batchNoPrefix+dateStamp)
	if err != nil {
		return "", nil, err
	}
	revisionSeq, err := s.repo.NextRevisionSequence(ctx, revisionNoPrefix+dateStamp)
	if err != nil {
		return "", nil, err
	}

	batchNo := fmt.Sprintf("%s%s%04d", batchNoPrefix, dateStamp, batchSeq+bump)
	revisions := make([]RepairRevision, 0, len(plans))
	for index, plan := range plans {
		revisions = append(revisions, RepairRevision{
			RevisionNo:     fmt.Sprintf("%s%s%04d", revisionNoPrefix, dateStamp, revisionSeq+bump+index),
			RepairID:       plan.repair.ID,
			RepairNo:       plan.repair.RepairNo,
			FaultID:        plan.repair.FaultID,
			RepairTeam:     plan.repair.RepairTeam,
			BatchNo:        batchNo,
			Seq:            plan.repair.RevisionCount + 1,
			OldResult:      plan.repair.EffectiveResult(),
			NewResult:      plan.newResult,
			OldCost:        plan.repair.EffectiveCost(),
			NewCost:        plan.newCost,
			Changes:        plan.changes,
			Reason:         reason,
			Operator:       operator,
			OriginMonth:    plan.originMonth,
			EffectiveMonth: plan.effectiveMonth,
			CrossSettled:   plan.crossSettled,
		})
	}
	return batchNo, revisions, nil
}
