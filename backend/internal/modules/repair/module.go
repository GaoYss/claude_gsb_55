package repair

import (
	"context"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module 维修记录模块, 负责维修过程录入与完工闭环。
type Module struct {
	repository *Repository
	service    *Service
	handler    *Handler
}

// New 构造维修记录模块, faults 为故障模块提供的端口实现。
func New(db *gorm.DB, faults FaultPort) *Module {
	repository := NewRepository(db)
	service := NewService(repository, faults)
	return &Module{
		repository: repository,
		service:    service,
		handler:    NewHandler(service),
	}
}

// Repository 暴露仓储, 供状态查询模块装配。
func (m *Module) Repository() *Repository { return m.repository }

// Name 实现 module.Module 接口。
func (m *Module) Name() string { return "维修记录" }

// Models 实现 module.Module 接口。
func (m *Module) Models() []any {
	return []any{&Repair{}, &RepairCorrection{}, &RepairMonthSettlement{}, &RepairSettlementRow{}}
}

// BackfillEffectiveResults 把历史已完工记录的生效结果初始化为原始结果, 供 bootstrap 在迁移后调用。
func (m *Module) BackfillEffectiveResults(ctx context.Context) error {
	return m.repository.BackfillEffectiveResults(ctx)
}

// RegisterRoutes 实现 module.Module 接口。
func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/repairs")
	{
		group.GET("", m.handler.List)
		group.POST("", m.handler.Create)
		group.GET("/meta", m.handler.Metadata)
		group.GET("/statistics", m.handler.Statistics)
		group.GET("/fault/:faultId", m.handler.ListByFault)
		group.POST("/corrections", m.handler.CorrectBatch)
		group.GET("/corrections", m.handler.ListCorrections)
		group.GET("/aggregation", m.handler.MonthlyAggregation)
		group.GET("/settlements", m.handler.ListSettlements)
		group.POST("/settlements", m.handler.SettleMonth)
		group.GET("/:id", m.handler.Get)
		group.PUT("/:id", m.handler.Update)
		group.POST("/:id/finish", m.handler.Finish)
		group.POST("/:id/correct", m.handler.Correct)
		group.GET("/:id/corrections", m.handler.ListRepairCorrections)
		group.DELETE("/:id", m.handler.Delete)
	}
}
