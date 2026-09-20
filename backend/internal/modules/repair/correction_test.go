package repair_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/repair"
)

// requireBadRequest 断言错误是 400 参数错误。
func requireBadRequest(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, http.StatusBadRequest, businessErr.Status, "错误信息: %s", businessErr.Message)
}

// requireBatchError 断言错误是批量更正校验失败, 并返回失败明细。
func requireBatchError(t *testing.T, err error) *repair.BatchCorrectError {
	t.Helper()
	require.Error(t, err)
	batchErr, ok := err.(*repair.BatchCorrectError)
	require.True(t, ok, "期望批量更正错误, 实际: %v", err)
	return batchErr
}

// backdateFault 登记一条指定上报时间的故障, 用于跨月场景。
func (h *harness) backdateFault(t *testing.T, lampID uint, reportedAt time.Time) *fault.Fault {
	t.Helper()
	entity, err := h.faults.Create(context.Background(), fault.CreateRequest{
		LampID:      lampID,
		FaultType:   "灯不亮",
		FaultLevel:  fault.LevelNormal,
		Source:      fault.SourceInspection,
		Description: "跨月更正测试",
		Reporter:    "巡检员",
		ReportedAt:  reportedAt.Format("2006-01-02 15:04:05"),
	})
	require.NoError(t, err)
	return entity
}

// finishRepair 开工并完工一条维修记录, 时间可回填。
func finishRepair(t *testing.T, h *harness, faultID uint, team, result string, cost float64, startedAt, finishedAt time.Time) *repair.Repair {
	t.Helper()
	ctx := context.Background()
	record, err := h.repairs.Create(ctx, repair.CreateRequest{
		FaultID:    faultID,
		Repairman:  "维修工甲",
		RepairTeam: team,
		StartedAt:  startedAt.Format("2006-01-02 15:04:05"),
	})
	require.NoError(t, err)
	finished, err := h.repairs.Finish(ctx, record.ID, repair.FinishRequest{
		Result:     result,
		Cost:       &cost,
		FinishedAt: finishedAt.Format("2006-01-02 15:04:05"),
	})
	require.NoError(t, err)
	return finished
}

// correctOne 更正单条维修记录。
func correctOne(t *testing.T, h *harness, repairID uint, result *string, cost *float64) *repair.CorrectResult {
	t.Helper()
	outcome, err := h.repairs.Correct(context.Background(), repair.CorrectRequest{
		Items:    []repair.CorrectItem{{RepairID: repairID, Result: result, Cost: cost}},
		Reason:   "现场复核后更正",
		Operator: "班组长",
	})
	require.NoError(t, err)
	return outcome
}

func strPtr(value string) *string    { return &value }
func floatPtr(value float64) *float64 { return &value }

func TestCorrectResultKeepsHistoryAndRefreshesStats(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-001")
	entity := h.backdateFault(t, device.ID, time.Now().Add(-3*time.Hour))

	record := finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultFixed, 210,
		time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour))

	faultAfterFinish, err := h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusRepaired, faultAfterFinish.Status)

	// 更正: 已修复 -> 待配件, 金额 210 -> 180
	outcome := correctOne(t, h, record.ID, strPtr(repair.ResultPendingParts), floatPtr(180))
	require.NotEmpty(t, outcome.BatchNo)
	require.Len(t, outcome.Revisions, 1)

	revision := outcome.Revisions[0]
	require.Regexp(t, `^XD\d{8}\d{4}$`, revision.RevisionNo)
	require.Regexp(t, `^PC\d{8}\d{4}$`, revision.BatchNo)
	require.Equal(t, 1, revision.Seq)
	require.Equal(t, repair.ResultFixed, revision.OldResult)
	require.Equal(t, repair.ResultPendingParts, revision.NewResult)
	require.Equal(t, 210.0, revision.OldCost)
	require.Equal(t, 180.0, revision.NewCost)
	require.Equal(t, "现场复核后更正", revision.Reason)
	require.Equal(t, "班组长", revision.Operator)
	require.False(t, revision.CrossSettled)
	require.Equal(t, revision.OriginMonth, revision.EffectiveMonth)

	// 逐字段对照: 结果与金额各一条
	require.Len(t, revision.Changes, 2)
	require.Equal(t, "result", revision.Changes[0].Field)
	require.Equal(t, "已修复", revision.Changes[0].OldValue)
	require.Equal(t, "待配件", revision.Changes[0].NewValue)
	require.Equal(t, "cost", revision.Changes[1].Field)
	require.Equal(t, "210.00", revision.Changes[1].OldValue)
	require.Equal(t, "180.00", revision.Changes[1].NewValue)

	// 历史留痕: 原结果、完工时间、原金额不被改写
	after, err := h.repairs.Get(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultFixed, after.Result)
	require.Equal(t, record.FinishedAt.Unix(), after.FinishedAt.Unix())
	require.Equal(t, 210.0, after.Cost)
	// 生效值切换到最新结果
	require.Equal(t, repair.ResultPendingParts, after.CurrentResult)
	require.Equal(t, 180.0, after.CurrentCost)
	require.Equal(t, 1, after.RevisionCount)

	// 修订链可从库中读回(序列化往返)
	chain, err := h.repairs.ListRevisionsByRepair(ctx, record.ID)
	require.NoError(t, err)
	require.Len(t, chain, 1)
	require.Equal(t, revision.RevisionNo, chain[0].RevisionNo)
	require.Len(t, chain[0].Changes, 2)

	// 故障状态联动: 已修复 -> 维修中
	faultAfter, err := h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusProcessing, faultAfter.Status)

	// 统计口径按最新结果刷新
	stats, err := h.repairs.Statistics(ctx)
	require.NoError(t, err)
	require.Equal(t, 180.0, stats.TotalCost)
	counts := make(map[string]int64, len(stats.ByResult))
	for _, bucket := range stats.ByResult {
		counts[bucket.Result] = bucket.Count
	}
	require.Equal(t, int64(1), counts[repair.ResultPendingParts])
	require.Equal(t, int64(0), counts[repair.ResultFixed])
}

func TestCorrectBatchIsAtomic(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-002")
	entity := h.backdateFault(t, device.ID, time.Now().Add(-4*time.Hour))

	finished := finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultPendingParts, 100,
		time.Now().Add(-3*time.Hour), time.Now().Add(-2*time.Hour))

	// 再登记一条进行中的维修记录(返修)
	ongoing, err := h.repairs.Create(ctx, repair.CreateRequest{FaultID: entity.ID, Repairman: "维修工乙"})
	require.NoError(t, err)

	// 批量更正: 一条完工 + 一条进行中, 任一条不满足 -> 整批不动
	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items: []repair.CorrectItem{
			{RepairID: finished.ID, Result: strPtr(repair.ResultFixed)},
			{RepairID: ongoing.ID, Result: strPtr(repair.ResultFixed)},
		},
		Reason:   "批量更正",
		Operator: "班组长",
	})
	batchErr := requireBatchError(t, err)
	require.Len(t, batchErr.Items, 1)
	require.Equal(t, ongoing.ID, batchErr.Items[0].RepairID)
	require.Contains(t, batchErr.Items[0].Message, "尚未完工")

	// 整批未执行: 完工记录的生效值不变, 也没有产生修订记录
	after, err := h.repairs.Get(ctx, finished.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultPendingParts, after.CurrentResult)
	require.Equal(t, 0, after.RevisionCount)
	chain, err := h.repairs.ListRevisionsByRepair(ctx, finished.ID)
	require.NoError(t, err)
	require.Empty(t, chain)
}

func TestCorrectValidation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-003")
	entity := h.backdateFault(t, device.ID, time.Now().Add(-3*time.Hour))

	record := finishRepair(t, h, entity.ID, "市政照明二班", repair.ResultObserving, 50,
		time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour))

	// 未提供任何更正字段
	_, err := h.repairs.Correct(ctx, repair.CorrectRequest{
		Items: []repair.CorrectItem{{RepairID: record.ID}}, Reason: "测试", Operator: "班组长",
	})
	batchErr := requireBatchError(t, err)
	require.Contains(t, batchErr.Items[0].Message, "至少更正一项")

	// 更正内容与原生效值一致
	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items:    []repair.CorrectItem{{RepairID: record.ID, Result: strPtr(repair.ResultObserving)}},
		Reason:   "测试",
		Operator: "班组长",
	})
	batchErr = requireBatchError(t, err)
	require.Contains(t, batchErr.Items[0].Message, "未发生变化")

	// 同一批次重复更正同一条记录
	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items: []repair.CorrectItem{
			{RepairID: record.ID, Result: strPtr(repair.ResultFixed)},
			{RepairID: record.ID, Result: strPtr(repair.ResultUnfixable)},
		},
		Reason:   "测试",
		Operator: "班组长",
	})
	batchErr = requireBatchError(t, err)
	require.Contains(t, batchErr.Items[0].Message, "重复更正")

	// 不存在的维修记录
	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items:    []repair.CorrectItem{{RepairID: 99999, Result: strPtr(repair.ResultFixed)}},
		Reason:   "测试",
		Operator: "班组长",
	})
	batchErr = requireBatchError(t, err)
	require.Contains(t, batchErr.Items[0].Message, "不存在")

	// 更正原因与操作人必填
	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items: []repair.CorrectItem{{RepairID: record.ID, Result: strPtr(repair.ResultFixed)}}, Operator: "班组长",
	})
	requireBadRequest(t, err)
	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items: []repair.CorrectItem{{RepairID: record.ID, Result: strPtr(repair.ResultFixed)}}, Reason: "测试",
	})
	requireBadRequest(t, err)
}

func TestCorrectChainAndClosedFault(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-004")
	entity := h.backdateFault(t, device.ID, time.Now().Add(-4*time.Hour))

	record := finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultFixed, 300,
		time.Now().Add(-3*time.Hour), time.Now().Add(-2*time.Hour))

	// 第一次更正: 已修复 -> 观察中
	correctOne(t, h, record.ID, strPtr(repair.ResultObserving), nil)
	faultAfter, err := h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusProcessing, faultAfter.Status)

	// 第二次更正: 观察中 -> 已修复, 修订链累计
	correctOne(t, h, record.ID, strPtr(repair.ResultFixed), nil)
	chain, err := h.repairs.ListRevisionsByRepair(ctx, record.ID)
	require.NoError(t, err)
	require.Len(t, chain, 2)
	require.Equal(t, 1, chain[0].Seq)
	require.Equal(t, 2, chain[1].Seq)
	require.Equal(t, repair.ResultObserving, chain[1].OldResult, "第二次修订应以上次生效值为旧值")
	require.Equal(t, repair.ResultFixed, chain[1].NewResult)

	faultAfter, err = h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusRepaired, faultAfter.Status)

	// 关闭故障后再更正: 修订照记, 故障不被重开
	_, err = h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "复核闭环"})
	require.NoError(t, err)
	correctOne(t, h, record.ID, strPtr(repair.ResultUnfixable), nil)
	faultAfter, err = h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusClosed, faultAfter.Status, "已关闭故障不随更正重开")

	after, err := h.repairs.Get(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultUnfixable, after.CurrentResult)
	require.Equal(t, 3, after.RevisionCount)
}

func TestCorrectOlderRepairDoesNotMoveFault(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-005")
	entity := h.backdateFault(t, device.ID, time.Now().Add(-6*time.Hour))

	// 第一次维修待配件, 故障保持维修中; 第二次返修已修复, 故障已修复
	first := finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultPendingParts, 80,
		time.Now().Add(-5*time.Hour), time.Now().Add(-4*time.Hour))
	finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultFixed, 120,
		time.Now().Add(-3*time.Hour), time.Now().Add(-2*time.Hour))

	faultAfter, err := h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusRepaired, faultAfter.Status)

	// 更正较早的维修记录: 不影响故障状态
	correctOne(t, h, first.ID, strPtr(repair.ResultObserving), nil)
	faultAfter, err = h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusRepaired, faultAfter.Status, "更正非最新维修记录不应联动故障状态")
}

func TestCrossMonthCorrectionKeepsSettledMonth(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-006")

	twoMonthsAgo := time.Now().AddDate(0, -2, 0)
	originMonth := twoMonthsAgo.Format("2006-01")
	currentMonth := time.Now().Format("2006-01")

	entity := h.backdateFault(t, device.ID, twoMonthsAgo.Add(-time.Hour))
	record := finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultFixed, 300,
		twoMonthsAgo, twoMonthsAgo.Add(time.Hour))

	// 结算原完工月, 归集金额冻结
	settlements, err := h.repairs.SettleMonth(ctx, repair.SettleRequest{Month: originMonth, Operator: "财务"})
	require.NoError(t, err)
	require.Len(t, settlements, 1)
	require.Equal(t, "市政照明一班", settlements[0].RepairTeam)
	require.Equal(t, int64(1), settlements[0].TotalCount)
	require.Equal(t, 300.0, settlements[0].TotalCost)

	// 已结算月份不允许重复结算
	_, err = h.repairs.SettleMonth(ctx, repair.SettleRequest{Month: originMonth, Operator: "财务"})
	requireConflict(t, err)

	// 不允许结算尚未结束的当前月
	_, err = h.repairs.SettleMonth(ctx, repair.SettleRequest{Month: currentMonth, Operator: "财务"})
	requireBadRequest(t, err)

	// 跨月更正: 已修复 -> 无法修复, 金额 300 -> 0
	outcome := correctOne(t, h, record.ID, strPtr(repair.ResultUnfixable), floatPtr(0))
	revision := outcome.Revisions[0]
	require.True(t, revision.CrossSettled, "原完工月已结算时应标记为跨月更正")
	require.Equal(t, originMonth, revision.OriginMonth)
	require.Equal(t, currentMonth, revision.EffectiveMonth, "跨月更正应计入当前开放月")

	// 已结算月的归集金额不受影响
	originAgg, err := h.repairs.Aggregation(ctx, repair.AggregationQuery{Month: originMonth})
	require.NoError(t, err)
	require.True(t, originAgg.Settled)
	require.Len(t, originAgg.Teams, 1)
	require.Equal(t, 300.0, originAgg.Teams[0].TotalCost, "已结算月份的归集金额不能被更正改写")
	require.Empty(t, originAgg.Adjustments, "调整项不应计入已结算的历史月")

	// 调整项计入当前开放月
	currentAgg, err := h.repairs.Aggregation(ctx, repair.AggregationQuery{Month: currentMonth})
	require.NoError(t, err)
	require.False(t, currentAgg.Settled)
	require.Len(t, currentAgg.Adjustments, 1)
	require.Equal(t, revision.RevisionNo, currentAgg.Adjustments[0].RevisionNo)
	require.Equal(t, originMonth, currentAgg.Adjustments[0].OriginMonth)
	require.Equal(t, -300.0, currentAgg.Adjustments[0].CostDelta)

	// 全局统计仍按最新结果实时刷新
	stats, err := h.repairs.Statistics(ctx)
	require.NoError(t, err)
	require.Equal(t, 0.0, stats.TotalCost)
}

func TestCrossMonthCorrectionRejectedWhenNoOpenMonth(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-007")

	twoMonthsAgo := time.Now().AddDate(0, -2, 0)
	originMonth := twoMonthsAgo.Format("2006-01")
	currentMonth := time.Now().Format("2006-01")

	entity := h.backdateFault(t, device.ID, twoMonthsAgo.Add(-time.Hour))
	record := finishRepair(t, h, entity.ID, "市政照明一班", repair.ResultFixed, 300,
		twoMonthsAgo, twoMonthsAgo.Add(time.Hour))

	_, err := h.repairs.SettleMonth(ctx, repair.SettleRequest{Month: originMonth, Operator: "财务"})
	require.NoError(t, err)

	// 模拟当前月也已结算(结算接口本身不允许结算当前月, 直接落库构造)
	require.NoError(t, h.db.Create(&repair.TeamMonthSettlement{
		Month: currentMonth, RepairTeam: "市政照明一班", SettledBy: "财务", SettledAt: time.Now(),
	}).Error)

	_, err = h.repairs.Correct(ctx, repair.CorrectRequest{
		Items:    []repair.CorrectItem{{RepairID: record.ID, Result: strPtr(repair.ResultUnfixable)}},
		Reason:   "跨月更正",
		Operator: "班组长",
	})
	batchErr := requireBatchError(t, err)
	require.Contains(t, batchErr.Items[0].Message, "没有可登记调整的开放月份")
}

func TestRevisionQueries(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-008")
	entity := h.backdateFault(t, device.ID, time.Now().Add(-3*time.Hour))

	record := finishRepair(t, h, entity.ID, "市政照明二班", repair.ResultFixed, 90,
		time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour))
	outcome := correctOne(t, h, record.ID, strPtr(repair.ResultPendingParts), floatPtr(60))
	revision := outcome.Revisions[0]

	// 按批次号查询
	items, total, _, err := h.repairs.ListRevisions(ctx, repair.RevisionListQuery{BatchNo: outcome.BatchNo})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, revision.RevisionNo, items[0].RevisionNo)

	// 按生效月查询
	_, total, _, err = h.repairs.ListRevisions(ctx, repair.RevisionListQuery{Month: time.Now().Format("2006-01")})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)

	// 详情含逐字段对照
	detail, err := h.repairs.GetRevision(ctx, revision.ID)
	require.NoError(t, err)
	require.Len(t, detail.Changes, 2)
	require.Equal(t, "维修结果", detail.Changes[0].Label)

	// 非法月份参数
	_, _, _, err = h.repairs.ListRevisions(ctx, repair.RevisionListQuery{Month: "2026-13"})
	requireBadRequest(t, err)
}
