import request from './request'

// 维修记录接口。
export const repairApi = {
  list: (params) => request.get('/repairs', { params }),
  detail: (id) => request.get(`/repairs/${id}`),
  listByFault: (faultId) => request.get(`/repairs/fault/${faultId}`),
  create: (data) => request.post('/repairs', data),
  update: (id, data) => request.put(`/repairs/${id}`, data),
  finish: (id, data) => request.post(`/repairs/${id}/finish`, data),
  remove: (id) => request.delete(`/repairs/${id}`),
  meta: () => request.get('/repairs/meta'),
  statistics: () => request.get('/repairs/statistics'),
  // 结果更正: 单条走 /:id/correct, 批量走 /corrections(整批原子)。
  correct: (id, data) => request.post(`/repairs/${id}/correct`, data),
  correctBatch: (data) => request.post('/repairs/corrections', data),
  // 修订记录: 逐字段对照的追溯链路。
  revisions: (params) => request.get('/repairs/revisions', { params }),
  revisionDetail: (id) => request.get(`/repairs/revisions/${id}`),
  repairRevisions: (id) => request.get(`/repairs/${id}/revisions`),
  // 班组月度归集与结算。
  aggregation: (params) => request.get('/repairs/aggregation', { params }),
  settlements: (params) => request.get('/repairs/settlements', { params }),
  settle: (data) => request.post('/repairs/settlements', data),
}
