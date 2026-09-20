package repair

import (
	"github.com/gin-gonic/gin"

	"streetlight/internal/apperr"
	"streetlight/internal/httpx"
	"streetlight/internal/response"
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

// Correct 单笔结果更正, 等价于长度为 1 的原子批量更正。
func (h *Handler) Correct(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req struct {
		Operator string `json:"operator" binding:"required,max=64"`
		CorrectItem
	}
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	result, err := h.service.Correct(c.Request.Context(), id, req.CorrectItem, req.Operator)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// CorrectBatch 批量结果更正, mode=atomic 整批原子(默认), mode=partial 逐条处理。
func (h *Handler) CorrectBatch(c *gin.Context) {
	var req CorrectBatchRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	result, err := h.service.CorrectBatch(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// ListCorrections 分页查询更正记录, 每条附带更正前后的逐字段对照。
func (h *Handler) ListCorrections(c *gin.Context) {
	var query CorrectionListQuery
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, page, err := h.service.ListCorrections(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.NewPageData(items, total, page.Page, page.PageSize))
}

// ListRepairCorrections 查询某条维修记录的完整修订路径。
func (h *Handler) ListRepairCorrections(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.service.ListRepairCorrections(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// MonthlyAggregation 查询某月的班组归集(未结算实时计算, 已结算返回封存快照)。
func (h *Handler) MonthlyAggregation(c *gin.Context) {
	month := c.Query("month")
	if month == "" {
		response.Fail(c, apperr.BadRequest("缺少查询参数: month(YYYY-MM)"))
		return
	}
	aggregation, err := h.service.MonthlyAggregation(c.Request.Context(), month)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, aggregation)
}

// SettleMonth 结算某月, 封存该月班组归集快照。
func (h *Handler) SettleMonth(c *gin.Context) {
	var req SettleMonthRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	aggregation, err := h.service.SettleMonth(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, aggregation)
}

// ListSettlements 查询全部已结算月份。
func (h *Handler) ListSettlements(c *gin.Context) {
	items, err := h.service.ListSettlements(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}
