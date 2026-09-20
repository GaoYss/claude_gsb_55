package repair_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/internal/modules/repair"
)

// finishRepair 走完整流程登记一条已完工维修记录: 路灯 -> 故障 -> 开工 -> 完工。
func finishRepair(t *testing.T, h *harness, code, team, result string, cost float64, reported, started, finished string) *repair.Repair {
	t.Helper()
	ctx := context.Background()

	device, err := h.lamps.Create(ctx, lamp.CreateRequest{
		Code: code, Name: "测试灯杆", RoadName: "测试路", LampType: lamp.LampTypeLED,
	})
	require.NoError(t, err)

	faultReq := fault.CreateRequest{
		LampID: device.ID, FaultType: "灯不亮", FaultLevel: fault.LevelNormal,
		Source: fault.SourceInspection, Description: "更正测试", Reporter: "巡检员",
	}
	if reported != "" {
		faultReq.ReportedAt = reported
	}
	faultEntity, err := h.faults.Create(ctx, faultReq)
	require.NoError(t, err)

	repairReq := repair.CreateRequest{FaultID: faultEntity.ID, Repairman: "维修工甲", RepairTeam: team}
	if started != "" {
		repairReq.StartedAt = started
	}
	record, err := h.repairs.Create(ctx, repairReq)
	require.NoError(t, err)

	finishReq := repair.FinishRequest{Result: result, Cost: &cost}
	if finished != "" {
		finishReq.FinishedAt = finished
	}
	finishedRecord, err := h.repairs.Finish(ctx, record.ID, finishReq)
	require.NoError(t, err)
	return finishedRecord
}

// monthDates 生成指定偏移月份内的三个时间点(上报/开工/完工), 格式 YYYY-MM-DD HH:mm:ss。
func monthDates(monthOffset int) (reported, started, finished, month string) {
	base := time.Now().AddDate(0, monthOffset, 0)
	reported = time.Date(base.Year(), base.Month(), 10, 8, 0, 0, 0, time.Local).Format("2006-01-02 15:04:05")
	started = time.Date(base.Year(), base.Month(), 11, 8, 0, 0, 0, time.Local).Format("2006-01-02 15:04:05")
	finished = time.Date(base.Year(), base.Month(), 12, 18, 0, 0, 0, time.Local).Format("2006-01-02 15:04:05")
	month = time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01")
	return
}

func currentMonth() string {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01")
}

// requireBadRequest 断言错误是 400 参数错误。
func requireBadRequest(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, http.StatusBadRequest, businessErr.Status, "错误信息: %s", businessErr.Message)
}

func TestFinishInitializesEffectiveResult(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	record := finishRepair(t, h, "LD-C-001", "市政照明一班", repair.ResultFixed, 120, "", "", "")
	require.Equal(t, repair.ResultFixed, record.CurrentResult)
	require.Equal(t, repair.LiabilityMaintenanceTeam, record.CurrentLiability)
	require.Equal(t, repair.NatureFaultRepair, record.CurrentNature)
	require.Equal(t, 0, record.CorrectionCount)

	// 列表的结果筛选按生效结果统计
	items, total, _, err := h.repairs.List(ctx, repair.ListQuery{Result: repair.ResultFixed})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, record.ID, items[0].ID)

	items, _, _, err = h.repairs.List(ctx, repair.ListQuery{Result: repair.ResultUnfixable})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestCorrectResultKeepsHistoryAndWritesRevision(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	reported, started, finished, _ := monthDates(-1)
	record := finishRepair(t, h, "LD-C-002", "市政照明一班", repair.ResultFixed, 210, reported, started, finished)
	originalFinishedAt := *record.FinishedAt

	result, err := h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultUnfixable,
		Reason:    "现场复核为灯杆老化需整体更换",
	}, "班长甲")
	require.NoError(t, err)
	require.Equal(t, repair.CorrectModeAtomic, result.Mode)
	require.Equal(t, 1, result.Applied)
	require.Regexp(t, `^ZG\d{8}\d{4}$`, result.BatchNo)
	require.Len(t, result.Succeeded, 1)

	correction := result.Succeeded[0]
	require.Equal(t, record.ID, correction.RepairID)
	require.Equal(t, repair.ResultFixed, correction.OldResult)
	require.Equal(t, repair.ResultUnfixable, correction.NewResult)
	require.Equal(t, repair.LiabilityMaintenanceTeam, correction.OldLiability)
	require.Equal(t, repair.LiabilityAssetOwner, correction.NewLiability)
	require.Equal(t, repair.NatureFaultRepair, correction.OldNature)
	require.Equal(t, repair.NatureRetrofit, correction.NewNature)
	require.Equal(t, 210.0, correction.Cost)
	require.Equal(t, "班长甲", correction.Operator)
	require.Equal(t, currentMonth(), correction.EffectMonth)

	// 逐字段对照: 结果 / 责任方 / 维修性质 三个字段的前后值
	require.Len(t, correction.Changes, 3)
	require.Equal(t, "result", correction.Changes[0].Field)
	require.Equal(t, "fixed", correction.Changes[0].Before)
	require.Equal(t, "unfixable", correction.Changes[0].After)
	require.Equal(t, "已修复", correction.Changes[0].BeforeLabel)
	require.Equal(t, "无法修复", correction.Changes[0].AfterLabel)
	require.Equal(t, "liability", correction.Changes[1].Field)
	require.Equal(t, "nature", correction.Changes[2].Field)

	// 历史记录上的原始结果与完工时间不被改写
	after, err := h.repairs.Get(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultFixed, after.Result, "原始结果必须保留")
	require.Equal(t, repair.ResultUnfixable, after.CurrentResult)
	require.Equal(t, repair.LiabilityAssetOwner, after.CurrentLiability)
	require.Equal(t, repair.NatureRetrofit, after.CurrentNature)
	require.Equal(t, 1, after.CorrectionCount)
	require.NotNil(t, after.LastCorrectedAt)
	require.True(t, originalFinishedAt.Equal(*after.FinishedAt), "完工时间不允许被更正改写")

	// 故障状态不受结果更正影响(已修复保持不变)
	faultEntity, err := h.faults.GetByID(ctx, record.FaultID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusRepaired, faultEntity.Status)

	// 修订路径可追溯: 再次更正后形成链条
	_, err = h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultObserving,
		Reason:    "更换后仍需观察一个周期",
	}, "班长甲")
	require.NoError(t, err)

	path, err := h.repairs.ListRepairCorrections(ctx, record.ID)
	require.NoError(t, err)
	require.Len(t, path, 2)
	require.Equal(t, repair.ResultFixed, path[0].OldResult)
	require.Equal(t, repair.ResultUnfixable, path[0].NewResult)
	require.Equal(t, repair.ResultUnfixable, path[1].OldResult)
	require.Equal(t, repair.ResultObserving, path[1].NewResult, "后一次更正应以前一次的生效结果为起点")

	final, err := h.repairs.Get(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultFixed, final.Result)
	require.Equal(t, repair.ResultObserving, final.CurrentResult)
	require.Equal(t, 2, final.CorrectionCount)
}

func TestCorrectValidation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	// 未完工记录不能更正
	device, err := h.lamps.Create(ctx, lamp.CreateRequest{Code: "LD-C-003", Name: "测试灯杆", RoadName: "测试路", LampType: lamp.LampTypeLED})
	require.NoError(t, err)
	faultEntity, err := h.faults.Create(ctx, fault.CreateRequest{
		LampID: device.ID, FaultType: "灯不亮", Description: "更正校验", Reporter: "巡检员",
	})
	require.NoError(t, err)
	ongoing, err := h.repairs.Create(ctx, repair.CreateRequest{FaultID: faultEntity.ID, Repairman: "维修工甲"})
	require.NoError(t, err)

	_, err = h.repairs.Correct(ctx, ongoing.ID, repair.CorrectItem{NewResult: repair.ResultFixed, Reason: "test"}, "班长甲")
	requireConflict(t, err)

	// 完工后: 与当前结果一致 -> 409; 原因为空 -> 409(整批拒绝); 非法结果 -> 409(整批拒绝)
	record := finishRepair(t, h, "LD-C-004", "市政照明二班", repair.ResultPendingParts, 80, "", "", "")

	_, err = h.repairs.Correct(ctx, record.ID, repair.CorrectItem{NewResult: repair.ResultPendingParts, Reason: "结果未变化"}, "班长甲")
	requireConflict(t, err)

	_, err = h.repairs.Correct(ctx, record.ID, repair.CorrectItem{NewResult: repair.ResultFixed, Reason: "  "}, "班长甲")
	requireConflict(t, err)

	_, err = h.repairs.CorrectBatch(ctx, repair.CorrectBatchRequest{
		Operator: "班长甲",
		Items:    []repair.CorrectItem{{RepairID: record.ID, NewResult: "not_a_result", Reason: "非法结果"}},
	})
	requireConflict(t, err)

	// 操作人为空 -> 400
	_, err = h.repairs.CorrectBatch(ctx, repair.CorrectBatchRequest{
		Operator: " ",
		Items:    []repair.CorrectItem{{RepairID: record.ID, NewResult: repair.ResultFixed, Reason: "原因"}},
	})
	requireBadRequest(t, err)

	// 不存在的维修记录 -> 整批拒绝
	_, err = h.repairs.CorrectBatch(ctx, repair.CorrectBatchRequest{
		Operator: "班长甲",
		Items:    []repair.CorrectItem{{RepairID: 99999, NewResult: repair.ResultFixed, Reason: "原因"}},
	})
	requireConflict(t, err)
}

func TestCorrectBatchAtomicAllOrNothing(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	first := finishRepair(t, h, "LD-C-005", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	second := finishRepair(t, h, "LD-C-006", "市政照明二班", repair.ResultObserving, 60, "", "", "")

	// 第二条不满足条件(与当前结果一致), 整批不动
	_, err := h.repairs.CorrectBatch(ctx, repair.CorrectBatchRequest{
		Operator: "班长甲",
		Items: []repair.CorrectItem{
			{RepairID: first.ID, NewResult: repair.ResultUnfixable, Reason: "第一条本应成功"},
			{RepairID: second.ID, NewResult: repair.ResultObserving, Reason: "与当前结果一致"},
		},
	})
	requireConflict(t, err)

	afterFirst, err := h.repairs.Get(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultFixed, afterFirst.CurrentResult, "原子模式下第一条也不允许落库")
	require.Equal(t, 0, afterFirst.CorrectionCount)

	corrections, err := h.repairs.ListRepairCorrections(ctx, first.ID)
	require.NoError(t, err)
	require.Empty(t, corrections, "整批拒绝时不应产生任何更正记录")
}

func TestCorrectBatchPartialProcessesPerItem(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	first := finishRepair(t, h, "LD-C-007", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	second := finishRepair(t, h, "LD-C-008", "市政照明二班", repair.ResultObserving, 60, "", "", "")

	result, err := h.repairs.CorrectBatch(ctx, repair.CorrectBatchRequest{
		Operator: "班长甲",
		Mode:     repair.CorrectModePartial,
		Items: []repair.CorrectItem{
			{RepairID: first.ID, NewResult: repair.ResultUnfixable, Reason: "复核后确认无法修复"},
			{RepairID: second.ID, NewResult: repair.ResultObserving, Reason: "与当前结果一致"},
			{RepairID: 99999, NewResult: repair.ResultFixed, Reason: "记录不存在"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, repair.CorrectModePartial, result.Mode)
	require.Equal(t, 1, result.Applied)
	require.Len(t, result.Succeeded, 1)
	require.Len(t, result.Failed, 2)
	require.NotEmpty(t, result.BatchNo)

	require.Equal(t, first.ID, result.Succeeded[0].RepairID)
	require.Equal(t, repair.ResultUnfixable, result.Succeeded[0].NewResult)

	require.Equal(t, 1, result.Failed[0].Index)
	require.Equal(t, second.ID, result.Failed[0].RepairID)
	require.Equal(t, second.RepairNo, result.Failed[0].RepairNo)
	require.NotEmpty(t, result.Failed[0].Message)
	require.Equal(t, 2, result.Failed[1].Index)

	// 成功的一条已生效, 失败的一条保持原样
	afterFirst, err := h.repairs.Get(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultUnfixable, afterFirst.CurrentResult)
	afterSecond, err := h.repairs.Get(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, repair.ResultObserving, afterSecond.CurrentResult)
	require.Equal(t, 0, afterSecond.CorrectionCount)
}

func TestMonthlyAggregationRefreshesAfterCorrection(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	record := finishRepair(t, h, "LD-C-009", "市政照明一班", repair.ResultFixed, 150, "", "", "")
	month := currentMonth()

	// 更正前: 维修班组 / 故障维修
	before, err := h.repairs.MonthlyAggregation(ctx, month)
	require.NoError(t, err)
	require.False(t, before.Settled)
	require.Len(t, before.Rows, 1)
	require.Equal(t, "市政照明一班", before.Rows[0].RepairTeam)
	require.Equal(t, repair.LiabilityMaintenanceTeam, before.Rows[0].Liability)
	require.Equal(t, repair.NatureFaultRepair, before.Rows[0].Nature)
	require.Equal(t, int64(1), before.Rows[0].RepairCount)
	require.Equal(t, 150.0, before.Rows[0].TotalCost)

	_, err = h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultPendingParts, Reason: "缺件待料",
	}, "班长甲")
	require.NoError(t, err)

	// 更正后同步刷新: 物资供应 / 待料缓修
	after, err := h.repairs.MonthlyAggregation(ctx, month)
	require.NoError(t, err)
	require.Len(t, after.Rows, 1)
	require.Equal(t, repair.LiabilityMaterialSupply, after.Rows[0].Liability)
	require.Equal(t, repair.NaturePendingMaterial, after.Rows[0].Nature)
	require.Equal(t, int64(1), after.Rows[0].RepairCount)
	require.Equal(t, 150.0, after.Rows[0].TotalCost)
}

func TestCrossMonthCorrectionKeepsSettledMonth(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	reported, started, finished, prevMonth := monthDates(-1)
	record := finishRepair(t, h, "LD-C-010", "市政照明一班", repair.ResultFixed, 300, reported, started, finished)

	// 结算上个月
	settled, err := h.repairs.SettleMonth(ctx, repair.SettleMonthRequest{Month: prevMonth, SettledBy: "财务乙"})
	require.NoError(t, err)
	require.True(t, settled.Settled)
	require.Equal(t, "财务乙", settled.SettledBy)
	require.Len(t, settled.Rows, 1)
	require.Equal(t, repair.LiabilityMaintenanceTeam, settled.Rows[0].Liability)
	require.Equal(t, 300.0, settled.Rows[0].TotalCost)

	// 跨月更正: 上月已修复 -> 无法修复
	_, err = h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultUnfixable, Reason: "灯杆整体锈蚀需更换",
	}, "班长甲")
	require.NoError(t, err)

	// 已结算月份的归集金额不受影响(快照封存)
	frozen, err := h.repairs.MonthlyAggregation(ctx, prevMonth)
	require.NoError(t, err)
	require.True(t, frozen.Settled)
	require.Len(t, frozen.Rows, 1)
	require.Equal(t, repair.LiabilityMaintenanceTeam, frozen.Rows[0].Liability)
	require.Equal(t, repair.NatureFaultRepair, frozen.Rows[0].Nature)
	require.Equal(t, int64(1), frozen.Rows[0].RepairCount)
	require.Equal(t, 300.0, frozen.Rows[0].TotalCost)
	require.Empty(t, frozen.Adjustments)

	// 调整计入更正发生月
	current, err := h.repairs.MonthlyAggregation(ctx, currentMonth())
	require.NoError(t, err)
	require.False(t, current.Settled)
	require.Empty(t, current.Rows)
	require.Len(t, current.Adjustments, 1)
	adjustment := current.Adjustments[0]
	require.Equal(t, record.ID, adjustment.RepairID)
	require.Equal(t, prevMonth, adjustment.SourceMonth)
	require.Equal(t, repair.LiabilityMaintenanceTeam, adjustment.OldLiability)
	require.Equal(t, repair.LiabilityAssetOwner, adjustment.NewLiability)
	require.Equal(t, 300.0, adjustment.Cost)

	// 更正记录上留有源月已结算的标记
	path, err := h.repairs.ListRepairCorrections(ctx, record.ID)
	require.NoError(t, err)
	require.Len(t, path, 1)
	require.True(t, path[0].SourceSettled)
	require.Equal(t, prevMonth, path[0].SourceMonth)
	require.Equal(t, currentMonth(), path[0].EffectMonth)
}

func TestSettleGuards(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	// 未来月份不能结算
	_, _, _, futureMonth := monthDates(1)
	_, err := h.repairs.SettleMonth(ctx, repair.SettleMonthRequest{Month: futureMonth, SettledBy: "财务乙"})
	requireBadRequest(t, err)

	// 非法月份格式
	_, err = h.repairs.SettleMonth(ctx, repair.SettleMonthRequest{Month: "2026-8", SettledBy: "财务乙"})
	requireBadRequest(t, err)

	// 先准备一条当月完工的记录, 再结算当月
	record := finishRepair(t, h, "LD-C-011", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	month := currentMonth()
	_, err = h.repairs.SettleMonth(ctx, repair.SettleMonthRequest{Month: month, SettledBy: "财务乙"})
	require.NoError(t, err)

	_, err = h.repairs.SettleMonth(ctx, repair.SettleMonthRequest{Month: month, SettledBy: "财务乙"})
	requireConflict(t, err)

	// 结算后: 该月不能新增完工记录
	ongoingFault := h.createFault(t, record.LampID, "结算后完工校验")
	ongoing, err := h.repairs.Create(ctx, repair.CreateRequest{FaultID: ongoingFault.ID, Repairman: "维修工乙"})
	require.NoError(t, err)
	_, err = h.repairs.Finish(ctx, ongoing.ID, repair.FinishRequest{Result: repair.ResultFixed})
	requireConflict(t, err)

	// 结算后: 更正因生效月已结算被拒绝
	_, err = h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultUnfixable, Reason: "生效月已结算",
	}, "班长甲")
	requireConflict(t, err)

	// 结算后: 该月完工记录不允许删除
	requireConflict(t, h.repairs.Delete(ctx, record.ID))

	settlements, err := h.repairs.ListSettlements(ctx)
	require.NoError(t, err)
	require.Len(t, settlements, 1)
	require.Equal(t, month, settlements[0].Month)
}

func TestDeleteRepairWithCorrectionsBlocked(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	record := finishRepair(t, h, "LD-C-012", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	_, err := h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultUnfixable, Reason: "复核更正",
	}, "班长甲")
	require.NoError(t, err)

	requireConflict(t, h.repairs.Delete(ctx, record.ID))
}

func TestCorrectionListQuery(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	record := finishRepair(t, h, "LD-C-013", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	batch, err := h.repairs.CorrectBatch(ctx, repair.CorrectBatchRequest{
		Operator: "班长甲",
		Items:    []repair.CorrectItem{{RepairID: record.ID, NewResult: repair.ResultUnfixable, Reason: "复核更正"}},
	})
	require.NoError(t, err)

	items, total, _, err := h.repairs.ListCorrections(ctx, repair.CorrectionListQuery{BatchNo: batch.BatchNo})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, record.RepairNo, items[0].RepairNo)

	_, total, _, err = h.repairs.ListCorrections(ctx, repair.CorrectionListQuery{Month: currentMonth()})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)

	_, _, _, err = h.repairs.ListCorrections(ctx, repair.CorrectionListQuery{Month: "2026-13"})
	requireBadRequest(t, err)
}

func TestStatisticsUseEffectiveResult(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	record := finishRepair(t, h, "LD-C-014", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	_, err := h.repairs.Correct(ctx, record.ID, repair.CorrectItem{
		NewResult: repair.ResultUnfixable, Reason: "复核更正",
	}, "班长甲")
	require.NoError(t, err)

	statistics, err := h.repairs.Statistics(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), statistics.ByResult[repair.ResultFixed])
	require.Equal(t, int64(1), statistics.ByResult[repair.ResultUnfixable])
	require.Equal(t, int64(1), statistics.ByLiability[repair.LiabilityAssetOwner])
	require.Equal(t, int64(1), statistics.ByNature[repair.NatureRetrofit])
}

// 保证批量更正的批次号在同一天内单调递增。
func TestCorrectionBatchNoSequence(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	first := finishRepair(t, h, "LD-C-015", "市政照明一班", repair.ResultFixed, 100, "", "", "")
	second := finishRepair(t, h, "LD-C-016", "市政照明二班", repair.ResultFixed, 100, "", "", "")

	batch1, err := h.repairs.Correct(ctx, first.ID, repair.CorrectItem{NewResult: repair.ResultObserving, Reason: "第一次"}, "班长甲")
	require.NoError(t, err)
	batch2, err := h.repairs.Correct(ctx, second.ID, repair.CorrectItem{NewResult: repair.ResultObserving, Reason: "第二次"}, "班长甲")
	require.NoError(t, err)
	require.NotEqual(t, batch1.BatchNo, batch2.BatchNo)
	require.True(t, batch2.BatchNo > batch1.BatchNo, fmt.Sprintf("批次号应递增: %s -> %s", batch1.BatchNo, batch2.BatchNo))
}
