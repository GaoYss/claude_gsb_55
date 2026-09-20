package repair

import (
	"errors"

	"github.com/gin-gonic/gin"

	"streetlight/internal/apperr"
	"streetlight/internal/httpx"
	"streetlight/internal/response"
	"streetlight/pkg/pagination"
)

// Handler 处理维修记录相关的 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 构造维修记录处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List 查询维修记录列表。
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, page, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.NewPageData(items, total, page.Page, page.PageSize))
}

// ListByFault 查询指定故障的维修过程记录。
func (h *Handler) ListByFault(c *gin.Context) {
	faultID, err := httpx.ParseID(c, "faultId")
	if err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.service.ListByFault(c.Request.Context(), faultID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Get 查询维修记录详情。
func (h *Handler) Get(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, entity)
}

// Create 录入维修记录。
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, entity)
}

// Update 修改维修记录。
func (h *Handler) Update(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req UpdateRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, entity)
}

// Finish 完成维修。
func (h *Handler) Finish(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req FinishRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.Finish(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, entity)
}

// Delete 删除维修记录。
func (h *Handler) Delete(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}

// Metadata 返回维修字典。
func (h *Handler) Metadata(c *gin.Context) {
	meta, err := h.service.Metadata(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, meta)
}

// Statistics 维修统计。
func (h *Handler) Statistics(c *gin.Context) {
	statistics, err := h.service.Statistics(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, statistics)
}

// CorrectOne 更正单条维修记录的结果, 等价于长度为 1 的批次。
func (h *Handler) CorrectOne(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req CorrectOneRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	result, err := h.service.Correct(c.Request.Context(), CorrectRequest{
		Items:    []CorrectItem{{RepairID: id, Result: req.Result, Cost: req.Cost}},
		Reason:   req.Reason,
		Operator: req.Operator,
	})
	if err != nil {
		failCorrect(c, err)
		return
	}
	response.Created(c, result)
}

// CorrectBatch 批量更正维修结果。
// 批量语义: 整批原子 —— 任一条目不满足条件时整批不执行, 409 响应逐条列出失败明细。
func (h *Handler) CorrectBatch(c *gin.Context) {
	var req CorrectRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	result, err := h.service.Correct(c.Request.Context(), req)
	if err != nil {
		failCorrect(c, err)
		return
	}
	response.Created(c, result)
}

// failCorrect 统一处理更正错误: 校验失败时返回 409 与逐条失败明细。
func failCorrect(c *gin.Context, err error) {
	var batchErr *BatchCorrectError
	if errors.As(err, &batchErr) {
		response.FailWithData(c,
			apperr.Conflict("更正未执行: %d 条记录不满足条件, 整批已取消", len(batchErr.Items)),
			gin.H{"items": batchErr.Items},
		)
		return
	}
	response.Fail(c, err)
}

// ListRevisions 分页查询修订记录。
func (h *Handler) ListRevisions(c *gin.Context) {
	var query RevisionListQuery
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, page, err := h.service.ListRevisions(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.NewPageData(items, total, page.Page, page.PageSize))
}

// GetRevision 查询单条修订记录详情(含逐字段对照)。
func (h *Handler) GetRevision(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	entity, err := h.service.GetRevision(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, entity)
}

// ListRepairRevisions 查询某维修记录的完整修订链。
func (h *Handler) ListRepairRevisions(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.service.ListRevisionsByRepair(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Aggregation 班组月度归集总览。
func (h *Handler) Aggregation(c *gin.Context) {
	var query AggregationQuery
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	result, err := h.service.Aggregation(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Settle 结算指定月份, 冻结该月各班组归集金额。
func (h *Handler) Settle(c *gin.Context) {
	var req SettleRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.service.SettleMonth(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, items)
}

// ListSettlements 分页查询结算快照。
func (h *Handler) ListSettlements(c *gin.Context) {
	var query pagination.Params
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, page, err := h.service.ListSettlements(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.NewPageData(items, total, page.Page, page.PageSize))
}
