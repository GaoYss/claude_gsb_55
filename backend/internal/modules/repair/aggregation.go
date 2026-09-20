package repair

import (
	"context"
	"sort"
	"strings"
	"time"

	"streetlight/internal/apperr"
	"streetlight/pkg/pagination"
)

// parseMonth 解析 YYYY-MM 月份, 返回当月第一天零点(本地时区)。
func parseMonth(value string) (time.Time, error) {
	month, err := time.ParseInLocation("2006-01", strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, apperr.BadRequest("月份格式应为 YYYY-MM, 当前值: %s", value)
	}
	return month, nil
}

// monthRange 返回月份对应的起止时间区间 [from, to)。
func monthRange(month time.Time) (time.Time, time.Time) {
	return month, month.AddDate(0, 1, 0)
}

// Aggregation 查询某月的班组归集总览。
// 未结算月按生效结果与生效金额实时归集; 已结算月返回冻结快照, 金额不再变化;
// 跨已结算月的更正以调整项计入其生效月, 不回写历史月。
func (s *Service) Aggregation(ctx context.Context, query AggregationQuery) (*MonthAggregation, error) {
	monthValue := strings.TrimSpace(query.Month)
	if monthValue == "" {
		monthValue = time.Now().Format("2006-01")
	}
	month, err := parseMonth(monthValue)
	if err != nil {
		return nil, err
	}
	monthKey := month.Format("2006-01")

	result := &MonthAggregation{
		Month:       monthKey,
		Teams:       make([]TeamAggregation, 0),
		Adjustments: make([]AdjustmentView, 0),
	}

	settlements, err := s.repo.SettlementsByMonth(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	if len(settlements) > 0 {
		result.Settled = true
		result.SettledBy = settlements[0].SettledBy
		settledAt := settlements[0].SettledAt
		result.SettledAt = &settledAt
		for _, row := range settlements {
			result.Teams = append(result.Teams, TeamAggregation{
				RepairTeam: row.RepairTeam,
				TotalCount: row.TotalCount,
				TotalCost:  row.TotalCost,
				ByResult:   toBucketViews(row.ByResult),
			})
		}
	} else {
		from, to := monthRange(month)
		rows, err := s.repo.AggregateFinishedByTeam(ctx, from, to)
		if err != nil {
			return nil, err
		}
		result.Teams = buildTeamAggregations(rows)
	}

	adjustments, err := s.repo.ListCrossSettledByMonth(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	for _, item := range adjustments {
		result.Adjustments = append(result.Adjustments, AdjustmentView{
			RevisionNo:  item.RevisionNo,
			RepairNo:    item.RepairNo,
			RepairTeam:  item.RepairTeam,
			OriginMonth: item.OriginMonth,
			OldResult:   item.OldResult,
			NewResult:   item.NewResult,
			CostDelta:   item.NewCost - item.OldCost,
			Reason:      item.Reason,
			Operator:      item.Operator,
			CreatedAt:   item.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// SettleMonth 结算指定月份: 冻结当月各班组的归集数量与金额。
// 只允许结算已结束的月份; 已结算的月份不允许重复结算。
func (s *Service) SettleMonth(ctx context.Context, req SettleRequest) ([]TeamMonthSettlement, error) {
	month, err := parseMonth(req.Month)
	if err != nil {
		return nil, err
	}
	monthKey := month.Format("2006-01")

	operator := strings.TrimSpace(req.Operator)
	if operator == "" {
		return nil, apperr.BadRequest("操作人不能为空")
	}

	currentFrom, _ := parseMonth(time.Now().Format("2006-01"))
	if !month.Before(currentFrom) {
		return nil, apperr.BadRequest("只能结算已结束的月份, %s 尚未结束", monthKey)
	}

	settled, err := s.repo.IsMonthSettled(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	if settled {
		return nil, apperr.Conflict("月份 %s 已完成结算, 不允许重复结算", monthKey)
	}

	from, to := monthRange(month)
	rows, err := s.repo.AggregateFinishedByTeam(ctx, from, to)
	if err != nil {
		return nil, err
	}
	teams := buildTeamAggregations(rows)

	now := time.Now()
	settlements := make([]TeamMonthSettlement, 0, len(teams))
	for _, team := range teams {
		buckets := make([]ResultBucket, 0, len(team.ByResult))
		for _, bucket := range team.ByResult {
			if bucket.Count == 0 {
				continue
			}
			buckets = append(buckets, ResultBucket{Result: bucket.Result, Count: bucket.Count, Cost: bucket.Cost})
		}
		settlements = append(settlements, TeamMonthSettlement{
			Month:      monthKey,
			RepairTeam: team.RepairTeam,
			TotalCount: team.TotalCount,
			TotalCost:  team.TotalCost,
			ByResult:   buckets,
			SettledBy:  operator,
			SettledAt:  now,
		})
	}

	if err := s.repo.CreateSettlements(ctx, settlements); err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.Conflict("月份 %s 已完成结算, 不允许重复结算", monthKey)
		}
		return nil, err
	}
	return settlements, nil
}

// ListSettlements 分页查询结算快照。
func (s *Service) ListSettlements(ctx context.Context, query pagination.Params) ([]TeamMonthSettlement, int64, pagination.Query, error) {
	page := pagination.Parse(query, pagination.SortSpec{Default: "month"})
	items, total, err := s.repo.ListSettlements(ctx, page)
	if err != nil {
		return nil, 0, page, err
	}
	return items, total, page, nil
}

// buildTeamAggregations 把 班组 × 结果 的明细行组装成班组归集视图。
// 班组为空时归入空班组键, 由前端展示为"未分配班组"。
func buildTeamAggregations(rows []TeamResultRow) []TeamAggregation {
	byTeam := make(map[string]*TeamAggregation)
	for _, row := range rows {
		team, ok := byTeam[row.RepairTeam]
		if !ok {
			team = &TeamAggregation{RepairTeam: row.RepairTeam, ByResult: make([]ResultBucketView, 0)}
			byTeam[row.RepairTeam] = team
		}
		team.TotalCount += row.Total
		team.TotalCost += row.Cost
		team.ByResult = append(team.ByResult, resultBucketView(row.Result, row.Total, row.Cost))
	}

	teams := make([]TeamAggregation, 0, len(byTeam))
	for _, team := range byTeam {
		sort.SliceStable(team.ByResult, func(i, j int) bool {
			return resultOrder(team.ByResult[i].Result) < resultOrder(team.ByResult[j].Result)
		})
		teams = append(teams, *team)
	}
	sort.SliceStable(teams, func(i, j int) bool {
		if teams[i].TotalCost == teams[j].TotalCost {
			return teams[i].RepairTeam < teams[j].RepairTeam
		}
		return teams[i].TotalCost > teams[j].TotalCost
	})
	return teams
}

// toBucketViews 把结算快照里的结果分组转换为带责任方与维修性质的视图。
func toBucketViews(buckets []ResultBucket) []ResultBucketView {
	views := make([]ResultBucketView, 0, len(buckets))
	for _, bucket := range buckets {
		views = append(views, resultBucketView(bucket.Result, bucket.Count, bucket.Cost))
	}
	sort.SliceStable(views, func(i, j int) bool {
		return resultOrder(views[i].Result) < resultOrder(views[j].Result)
	})
	return views
}

// resultBucketView 组装单个结果分组视图, 未登记的结果只保留原始取值。
func resultBucketView(result string, count int64, cost float64) ResultBucketView {
	view := ResultBucketView{Result: result, Count: count, Cost: cost}
	if meta, ok := ResultMetaOf(result); ok {
		view.ResultLabel = meta.Label
		view.LiableParty = meta.LiableParty
		view.RepairNature = meta.RepairNature
	}
	return view
}

// resultOrder 返回结果在展示中的排序位次, 未登记的结果排在最后。
func resultOrder(result string) int {
	for index, item := range Results() {
		if item == result {
			return index
		}
	}
	return len(Results())
}
